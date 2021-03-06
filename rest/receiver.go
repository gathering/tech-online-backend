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

/*
Package rest is scaffolding around net/http that facilitates a
RESTful HTTP API with certain patterns implicitly enforced:
- When working on the same urls, all Methods should use the exact same
data structures. E.g.: What you PUT is the same as what you GET out
again. No cheating.
- ETag is computed for all responses.
- All responses are JSON-encoded, including error messages.

See objects/thing.go for how to use this, but the essence is:
1. Make whatever data structure you need.
2. Implement one or more of gondulapi.Getter/Putter/Poster/Deleter.
3. Use AddHandler() to register that data structure on a URL path
4. Grab lunch.

Receiver tries to do all HTTP and caching-related tasks for you, so you
don't have to.
*/
package rest

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"

	"github.com/gathering/tech-online-backend/config"
	"github.com/google/uuid"
	log "github.com/sirupsen/logrus"
)

type receiver struct {
	pathPattern regexp.Regexp
	allocator   Allocator
}

type receiverSet struct {
	pathPrefix string
	receivers  []receiver
}

// Map of all receiver sets
var receiverSets map[string]*receiverSet

type input struct {
	requestID  uuid.UUID
	url        *url.URL
	pathPrefix string
	pathSuffix string
	method     string
	data       []byte
	query      map[string][]string
	pretty     bool
}

type output struct {
	code         int
	data         interface{}
	location     string
	cachecontrol string
}

// AddHandler registeres an allocator/data structure with a url. The
// allocator should be a function returning an empty datastrcuture which
// implements one or more of gondulapi.Getter, Putter, Poster and Deleter
func AddHandler(pathPrefix string, pathPattern string, allocator Allocator) error {
	if receiverSets == nil {
		receiverSets = make(map[string]*receiverSet)
	}

	var set *receiverSet
	if value, exists := receiverSets[pathPrefix]; exists {
		set = value
	} else {
		newSet := receiverSet{pathPrefix: pathPrefix}
		set = &newSet
		receiverSets[pathPrefix] = &newSet
	}

	var compiledPathPattern *regexp.Regexp
	if result, err := regexp.Compile(pathPattern); err == nil {
		compiledPathPattern = result
	} else {
		err := fmt.Errorf("invalid regexp pattern for path: %v", pathPattern)
		log.WithError(err).Error("failed to compile path pattern for handler")
		return err
	}

	receiver := receiver{*compiledPathPattern, allocator}
	set.receivers = append(set.receivers, receiver)
	return nil
}

// Allocator is used to allocate a data structure that implements at least
// one of Getter, Putter, Poster or Deleter from gondulapi.
type Allocator func() interface{}

// StartReceiver a net/http server and handle all requests registered. Never
// returns.
func StartReceiver() {
	var server http.Server
	serveMux := http.NewServeMux()
	server.Handler = serveMux
	server.Addr = ":8080"
	if config.Config.ListenAddress != "" {
		server.Addr = config.Config.ListenAddress
	}

	// Default handler, for consistent 404s
	defaultReceiverSet := receiverSet{pathPrefix: "/"}
	serveMux.Handle("/", defaultReceiverSet)

	// Receiver handlers
	for _, set := range receiverSets {
		set.pathPrefix = config.Config.SitePrefix + set.pathPrefix
		serveMux.Handle(set.pathPrefix, set)
		for _, receiver := range set.receivers {
			log.Infof("Added receiver [%v][%v]' for [%T].", set.pathPrefix, receiver.pathPattern.String(), receiver.allocator())
		}
	}

	log.WithFields(log.Fields{
		"listen_address": server.Addr,
		"path_prefix":    config.Config.SitePrefix,
	}).Info("Server is listening")
	log.Fatal(server.ListenAndServe())
}

func (set receiverSet) ServeHTTP(httpWriter http.ResponseWriter, httpRequest *http.Request) {
	requestID := uuid.New()
	log.WithFields(log.Fields{
		"id":     requestID,
		"url":    httpRequest.URL,
		"method": httpRequest.Method,
		"client": httpRequest.RemoteAddr,
	}).Infof("Request")

	// Process request content
	input, err := processInput(httpRequest, set.pathPrefix, requestID)
	if err != nil {
		log.WithFields(log.Fields{
			"data": string(input.data),
			"err":  err,
		}).Warn("Failed to process request input")
		return
	}

	// Purge expired access tokens
	// Should happen as periodic task, but whatever, requests are pretty periodic and this is pretty quick
	// TODO optimize
	purgeExpiredAccessTokens()

	// Load access token entry (if any valid) and user (if any associated)
	token := getRequestAccessToken(httpRequest)

	// Find matching receiver
	var foundReceiver *receiver
	for _, receiver := range set.receivers {
		if receiver.pathPattern.MatchString(input.pathSuffix) {
			log.WithFields(log.Fields{
				"prefix":  set.pathPrefix,
				"pattern": receiver.pathPattern.String(),
			}).Trace("Found receiver")
			foundReceiver = &receiver
			break
		}
	}

	// Handle request at appropriate endpoints
	result, data := handleRequest(foundReceiver, input, token)

	// Process output
	output := processOutput(input, result, data)

	// Create response
	sendResponse(httpWriter, input, output)
}

func getRequestAccessToken(httpRequest *http.Request) AccessTokenEntry {
	var token *AccessTokenEntry
	authHeader, authHeaderFound := httpRequest.Header["Authorization"]
	if authHeaderFound {
		authHeaderFields := strings.Fields(authHeader[0])
		if len(authHeaderFields) == 2 && strings.ToLower(authHeaderFields[0]) == "bearer" {
			tokenKey := authHeaderFields[1]
			token = loadAccessTokenByKey(tokenKey)
		}
	}
	// Ignore illegal or malformed token, just give them a guest token instead of complaining
	if token == nil {
		guestToken := makeGuestAccessToken()
		token = &guestToken
	}
	log.WithFields(log.Fields{
		"token":   token.ID,
		"role":    token.GetRole(),
		"comment": token.Comment,
	}).Trace("Using access token")

	return *token
}

// get is a badly named function in the context of HTTP since what it
// really does is just read the body of a HTTP request. In my defence, it
// used to do more. But what has it done for me lately?!
func processInput(httpRequest *http.Request, pathPrefix string, requestID uuid.UUID) (input, error) {
	var input input
	input.requestID = requestID
	fullPath := httpRequest.URL.Path
	// Make sure path always ends with "/"
	if !strings.HasSuffix(fullPath, "/") {
		fullPath += "/"
	}
	input.url = httpRequest.URL
	input.pathPrefix = pathPrefix
	input.pathSuffix = fullPath[len(pathPrefix):]
	input.query = httpRequest.URL.Query()
	input.method = httpRequest.Method
	input.pretty = len(httpRequest.URL.Query()["pretty"]) > 0

	// Process body
	if httpRequest.ContentLength != 0 {
		input.data = make([]byte, httpRequest.ContentLength)

		if n, err := io.ReadFull(httpRequest.Body, input.data); err != nil {
			log.WithFields(log.Fields{
				"address":  httpRequest.RemoteAddr,
				"error":    err,
				"numbytes": n,
			}).Error("Read error from client")
			return input, fmt.Errorf("read failed: %v", err)
		}
	}

	return input, nil
}

// handle figures out what Method the input has, casts item to the correct
// interface and calls the relevant function, if any, for that data. For
// PUT and POST it also parses the input data.
func handleRequest(receiver *receiver, input input, accessToken AccessTokenEntry) (result Result, data interface{}) {
	// No handler
	if receiver == nil {
		result.Code = 404
		result.Message = "endpoint not found"
		return
	}

	// Prepare request object
	var request Request
	request.ID = input.requestID
	request.Method = input.method
	request.AccessToken = accessToken
	request.PathArgs = make(map[string]string)
	argCaptures := receiver.pathPattern.FindStringSubmatch(input.pathSuffix)
	argCaptureNames := receiver.pathPattern.SubexpNames()
	for i := range argCaptures {
		if i > 0 {
			if argCaptureNames[i] != "" {
				request.PathArgs[argCaptureNames[i]] = argCaptures[i]
			}
		}
	}
	request.QueryArgs = make(map[string]string)
	for key, value := range input.query {
		// Only use first arg for each key
		if len(value) > 0 {
			request.QueryArgs[key] = value[0]
		} else {
			request.QueryArgs[key] = ""
		}
	}
	if value, exists := request.QueryArgs["limit"]; exists {
		if i, err := strconv.Atoi(value); err == nil {
			request.ListLimit = i
		}
	}
	if _, exists := request.QueryArgs["brief"]; exists {
		request.ListBrief = true
	}

	// Find handler and handle
	item := receiver.allocator()
	switch input.method {
	case "OPTIONS":
	case "HEAD":
		get, ok := item.(Getter)
		if !ok {
			result.Code = 405
			result.Message = "method not allowed for endpoint"
			return
		}
		result = get.Get(&request)
		data = nil
	case "GET":
		get, ok := item.(Getter)
		if !ok {
			result.Code = 405
			result.Message = "method not allowed for endpoint"
			return
		}
		result = get.Get(&request)
		data = get
	case "POST":
		if len(input.data) > 0 {
			if err := json.Unmarshal(input.data, &item); err != nil {
				log.WithError(err).Trace("Failed to unmarshal JSON for endpoint")
				result.Code = 400
				result.Message = "malformed data for endpoint"
				return
			}
		}
		post, ok := item.(Poster)
		if !ok {
			result.Code = 405
			result.Message = "method not allowed for endpoint"
			return
		}
		result = post.Post(&request)
		data = post
	case "PUT":
		if len(input.data) > 0 {
			if err := json.Unmarshal(input.data, &item); err != nil {
				log.WithError(err).Trace("Failed to unmarshal JSON for endpoint")
				result.Code = 400
				result.Message = "malformed data for endpoint"
				return
			}
		}
		put, ok := item.(Putter)
		if !ok {
			result.Code = 405
			result.Message = "method not allowed for endpoint"
			return
		}
		result = put.Put(&request)
	case "DELETE":
		del, ok := item.(Deleter)
		if !ok {
			result.Code = 405
			result.Message = "method not allowed for endpoint"
			return
		}
		result = del.Delete(&request)
	default:
		result.Code = 405
		result.Message = "method not allowed for endpoint"
		return
	}

	return
}

func processOutput(input input, result Result, handlerData interface{}) (output output) {
	if result.Error != nil {
		log.WithError(result.Error).Warn("internal server error")
		result.Code = 500
	}

	if result.Code != 0 {
		output.code = result.Code
	} else {
		output.code = 200
	}

	switch {
	case output.code >= 100 && output.code <= 199:
	case output.code >= 200 && output.code <= 299:
		// Data
		if output.code == 204 {
			// No data allowed
			output.data = nil
		} else if handlerData == nil {
			// Show report if no returned data
			output.data = result
		} else {
			// Show data
			output.data = handlerData
		}
		// Location
		if output.code == 201 {
			output.location = result.Location
		}
	case output.code >= 300 && output.code <= 399:
		// Hide data
		output.data = result
		output.location = result.Location
	case output.code >= 400 && output.code <= 499:
		// Always hide data on error
		output.data = result
	default:
		// Overwrite both code and data if something weird
		output.code = 500
		output.data = message("internal server error")
	}

	// OPTIONS and HEAD must never return data
	if input.method == "OPTIONS" || input.method == "HEAD" {
		output.data = nil
	}

	return
}

// answer replies to a HTTP request with the provided output, optionally
// formatting the output prettily. It also calculates an ETag.
func sendResponse(w http.ResponseWriter, input input, output output) {
	log.WithFields(log.Fields{
		"code":     output.code,
		"location": output.location,
	}).Trace("Request done")

	code := output.code

	// Content
	body := make([]byte, 0)
	if output.data != nil {
		var jsonErr error
		if input.pretty {
			body, jsonErr = json.MarshalIndent(output.data, "", "  ")
		} else {
			body, jsonErr = json.Marshal(output.data)
		}
		if jsonErr != nil {
			log.WithError(jsonErr).Error("Failed to marshal response data to JSON")
			code = 500
			body = make([]byte, 0)
		}
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
	}

	// CORS
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "*")
	w.Header().Set("Access-Control-Allow-Headers", "*")
	w.Header().Set("Access-Control-Max-Age", "300") // 5 minutes

	// Caching header
	etagraw := sha256.Sum256(body)
	etagstr := hex.EncodeToString(etagraw[:])
	w.Header().Set("ETag", etagstr)

	// Redirect
	if output.location != "" {
		w.Header().Set("Location", output.location)
	}

	// Finalize head and add body
	w.WriteHeader(code)
	if code != 204 {
		fmt.Fprintf(w, "%s\n", body)
	}
}

// message is a convenience function
func message(str string, v ...interface{}) (m struct {
	Message string `json:"message"`
}) {
	m.Message = fmt.Sprintf(str, v...)
	return
}
