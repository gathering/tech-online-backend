/*
Tech:Online Backend
Copyright 2020, Kristian Lyngstøl <kly@kly.no>
Copyright 2021-2022, Håvard Ose Nordstrand <hon@hon.one>

This program is free software; you can redistribute it and/or
modify it under the terms of the GNU General Public License
as published by the Free Software Foundation; either version 2
of the License, or (at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
GNU General Public License for more details.

You should have received a copy of the GNU General Public License
along with this program; if not, write to the Free Software
Foundation, Inc., 51 Franklin Street, Fifth Floor, Boston, MA  02110-1301, USA.
*/

package db

import (
	"fmt"
	"reflect"

	log "github.com/sirupsen/logrus"
)

// Get gets stuff, fails if not found.
func Get(item interface{}, table string, searcher ...interface{}) Result {
	result := Select(item, table, searcher...)
	if result.Error == nil && result.Ok == 0 {
		return Result{Error: newError("Couldn't find the item")}
	}
	return result
}

// Select populates the provided interface(should be a pointer to a struct)
// by performing a simple select on the table, matching haystack with needle.
// E.g: select (elements of d) from table where haystack = needle;
//
// It is not particularly fast, and uses reflection. It is well suited for
// simple objects, but if performance is important, roll your own.
//
// It only populates exported values, and will use the "column" tag as an
// alternate name if needed. The data fields are fetched with sql.Scan(),
// so the data types need to implement sql.Scan() somehow.
//
// If the element is found, "found" is returned as true. If the element is
// found, but fails to scan, found is returned as true, but err is is
// non-nil. If an error occurred before the query is executed, or with the
// query itself (e.g.: bad table, or database issues), found is returned as
// false, and error is set. As such: if found is true, you can trust it, if
// it is false, you can only be absolutely sure the element doesn't exist
// if err is false.
//
// It needs to do two passes (might be a better way if someone better at
// the inner workings of Go reflection than me steps up). The first pass
// iterates over the fields of d, preparing both the query and allocating
// zero-values of the relevant objects. After this, the query is executed
// and the values are stored on the temporary values. The last pass stores
func Select(d interface{}, table string, searcher ...interface{}) Result {
	st := reflect.ValueOf(d)
	if st.Kind() != reflect.Ptr {
		return Result{Error: newError("Select() called with non-pointer interface. This wouldn't really work.")}
	}
	st = reflect.Indirect(st)

	// Set up a slice for the response
	retv := reflect.MakeSlice(reflect.SliceOf(st.Type()), 0, 0)
	retvi := retv.Interface()

	// Do the actual work :D
	selectResult := SelectMany(&retvi, table, searcher...)
	if selectResult.Error != nil {
		return selectResult
	}

	// retvi will be overwritten with the response (because that's how
	// append works), so retv now points to the empty original - update
	// it.
	retv = reflect.ValueOf(retvi)
	if retv.Len() == 0 {
		return Result{}
	}
	reply := retv.Index(0)
	setthis := reflect.Indirect(reflect.ValueOf(d))
	setthis.Set(reply)
	return Result{Ok: 1}
}

// Selector TODO desc
type Selector struct {
	Haystack string
	Operator string
	Needle   interface{}
}

func buildWhere(offset int, search []Selector) (string, []interface{}) {
	strsearch := ""
	searcharr := make([]interface{}, 0)
	nextidx := 1
	for _, item := range search {
		var whereand string
		if strsearch == "" {
			whereand = "WHERE"
		} else {
			whereand = "AND"
		}
		if item.Needle == nil {
			strsearch = fmt.Sprintf("%s %s %s %s NULL", strsearch, whereand, item.Haystack, item.Operator)
		} else {
			strsearch = fmt.Sprintf("%s %s %s %s $%d", strsearch, whereand, item.Haystack, item.Operator, offset+nextidx)
			nextidx++
			searcharr = append(searcharr, item.Needle)
		}
	}
	return strsearch, searcharr
}

// SelectMany selects multiple rows from the table, populating the slice
// pointed to by d. It must, as such, be called with a pointer to a slice as
// the d-value (it checks).
//
// It returns the data in d, with an error if something failed, obviously.
// It's not particularly fast, or bullet proof, but:
//
// 1. It handles the needle safely, e.g., it lets the sql driver do the
// escaping.
//
// 2. The haystack and table is NOT safe.
//
// 3. It uses database/sql.Scan, so as long as your elements implement
// that, it will Just Work.
//
// It works by first determining the base object/type to fetch by digging
// into d with reflection. Once that is established, it iterates over the
// discovered base-structure and does two things: creates the list of
// columns to SELECT, and creates a reflect.Value for each column to store
// the result. Once this loop is done, it executes the query, then iterates
// over the replies, storing them in new base elements. At the very end,
// the *d is overwritten with the new slice.
func SelectMany(d interface{}, table string, searcher ...interface{}) Result {
	if DB == nil {
		return Result{Error: newError("Tried to issue SelectMany() without a DB object")}
	}
	dval := reflect.ValueOf(d)
	// This is needed because we need to be able to update with a
	// potentially new slice.
	if dval.Kind() != reflect.Ptr {
		return Result{Error: newError("SelectMany() called with non-pointer interface. This wouldn't really work. Got %T", d)}
	}
	dval = reflect.Indirect(dval)

	// This enables Select() to work, and generally masks over issues
	// where the type is obscured by layers of casting and conversion.
	if dval.Kind() == reflect.Interface {
		dval = dval.Elem()
	}
	// And obviously it needs to actually be a slice.
	if dval.Kind() != reflect.Slice {
		return Result{Error: newError("SelectMany() must be called with pointer-to-slice, e.g: &[]foo, got: %T inner is: %v / %#v / %s / kind: %s", d, dval, dval, dval, dval.Kind())}
	}

	search, err := buildSearch(searcher...)
	if err != nil {
		return Result{Error: err}
	}
	// st stores the type we need to return an array, while fieldList
	// stores the actual base element. Usually, they are the same,
	// unless you pass []*foo, in which case st will represent *foo and
	// fieldList will represent foo.
	st := dval.Type()
	st = st.Elem()
	fieldList := st
	if fieldList.Kind() == reflect.Ptr {
		fieldList = fieldList.Elem()
	}

	// We make a new slice - this is what we will actually return/set
	retv := reflect.MakeSlice(reflect.SliceOf(st), 0, 0)

	keys, comma := "", ""
	sample := reflect.New(fieldList)
	sampleUnderscoreRaw := sample.Interface()
	haystacks := make(map[string]bool, 0)
	kvs, err := enumerate(haystacks, true, &sampleUnderscoreRaw)
	if err != nil {
		return Result{Error: newErrorWithCause("enumerate() failed during query. This is bad.", err)}
	}
	for idx := range kvs.keys {
		keys = fmt.Sprintf("%s%s%s", keys, comma, kvs.keys[idx])
		comma = ","
	}
	strsearch, searcharr := buildWhere(0, search)
	q := fmt.Sprintf("SELECT %s FROM %s%s", keys, table, strsearch)
	log.WithField("query", q).Trace("Select()")
	rows, err := DB.Query(q, searcharr...)
	if err != nil {
		return Result{Error: newErrorWithCause("Select(): SELECT failed on DB.Query", err)}
	}
	defer func() {
		rows.Close()
	}()

	// Read the rows...
	numElements := 0
	for {
		ok := rows.Next()
		if !ok {
			break
		}

		err = rows.Scan(kvs.newvals...)
		if err != nil {
			return Result{Error: newErrorWithCause("Select(): SELECT failed to scan", err)}
		}

		// Create the new slice element
		newidx := reflect.New(st)
		newidx = reflect.Indirect(newidx)

		// If it's an array of pointers, we need to fiddle a bit.
		// This is probably not prefect.
		newval := newidx
		if newidx.Kind() == reflect.Ptr {
			newval = reflect.New(st.Elem()) // returns a _pointer_ to the new value, which is why this works.
			newidx.Set(newval)
			newval = reflect.Indirect(newval)
		}

		for idx := range kvs.newvals {
			newv := reflect.Indirect(reflect.ValueOf(kvs.newvals[idx]))
			value := newval.Field(kvs.keyidx[idx])
			value.Set(newv)
		}

		retv = reflect.Append(retv, newidx)
		numElements++
	}

	// Finally - store the new slice to the pointer provided as input
	setthis := reflect.Indirect(reflect.ValueOf(d))
	setthis.Set(retv)
	return Result{Ok: numElements}
}

// Exists checks if a row where haystack matches the needle exists on the
// given table. It returns found=true if it does. It returns found=false if
// it doesn't find it - including if an error occurs (which will also be
// returned).
func Exists(table string, searcher ...interface{}) Result {
	search, err := buildSearch(searcher...)
	if err != nil {
		return Result{Error: newErrorWithCause("Exists(): failed, unable to build search", err)}
	}
	searchstr, searcharr := buildWhere(0, search)
	q := fmt.Sprintf("SELECT * FROM %s WHERE %s LIMIT 1", table, searchstr)
	log.WithField("query", q).Trace("Exists()")
	rows, err := DB.Query(q, searcharr...)
	if err != nil {
		return Result{Error: newErrorWithCause("Exists(): SELECT failed", err)}
	}
	defer func() {
		rows.Close()
	}()
	ok := rows.Next()
	if !ok {
		return Result{}
	}
	return Result{Ok: 1}
}

// Get is a convenience-wrapper for Select that return suitable
// gondulapi-errors if the needle is the Zero-value, if the database-query
// fails or if the item isn't found.
//
// It is provided so callers can implement receiver.Getter by simply
// calling this to get reasonable default-behavior.
/*func Get(needle interface{}, haystack string, table string, item interface{}) error {
	value := reflect.ValueOf(needle)
	if value.IsZero() {
		return gondulapi.Errorf(400, "No item to look for provided")
	}
	found, err := Select([]Selector{{haystack,"=",needle}}, table, item)
	if err != nil {
		return gondulapi.InternalError
	}
	if !found {
		return gondulapi.Errorf(404, "Couldn't find item %v", needle)
	}
	return nil
}*/

func buildSearch(searcher ...interface{}) ([]Selector, error) {
	var search []Selector
	if len(searcher) == 0 {
		search = []Selector{}
	} else if len(searcher)%3 != 0 {
		return nil, newError("Uneven search function call")
	} else {
		search = make([]Selector, 0)
		for i := 0; i < len(searcher); i += 3 {
			search = append(search, Selector{searcher[i].(string), searcher[i+1].(string), searcher[i+2]})
		}
	}
	return search, nil
}
