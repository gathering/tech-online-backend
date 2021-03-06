/*
Tech:Online backend
Copyright 2020, Kristian Lyngstøl <kly@kly.no>
Copyright 2021, Håvard Ose Nordstrand <hon@hon.one>

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

package yolo

import (
	"database/sql"

	"github.com/gathering/tech-online-backend/db"
	"github.com/gathering/tech-online-backend/rest"
	"github.com/google/uuid"
)

// TrackStations consists of all stations for a track.
type TrackStations struct {
	ID       string    `json:"id"`
	Type     TrackType `json:"type"`
	Name     string    `json:"name"`
	Stations Stations  `json:"stations"`
}

// StationTasksTests consists of all tasks and tests for a track and station.
type StationTasksTests struct {
	ID               string                   `json:"id"`
	Type             TrackType                `json:"type"`
	Name             string                   `json:"name"`
	StationShortname string                   `json:"station_shortname"`
	Tasks            []*stationTasksTestsTask `json:"tasks"`
}

type stationTasksTestsTask struct {
	ID          *uuid.UUID `json:"id"`
	Shortname   string     `json:"shortname"`
	Name        string     `json:"name"`
	Description string     `json:"description"`
	Sequence    *int       `json:"sequence"`
	Tests       []Test     `json:"tests"`
}

func init() {
	rest.AddHandler("/custom/track-stations/", "^(?P<track_id>[^/]+)/$", func() interface{} { return &TrackStations{} })
	rest.AddHandler("/custom/station-tasks-tests/", "^(?P<track_id>[^/]+)/(?P<station_shortname>[^/]+)/$", func() interface{} { return &StationTasksTests{} })
}

// Get creates a a big mess of data consisting of a track and all non-terminated stations for it.
func (trackAndStations *TrackStations) Get(request *rest.Request) rest.Result {
	trackID, trackIDExists := request.PathArgs["track_id"]
	if !trackIDExists || trackID == "" {
		return rest.Result{Code: 400, Message: "missing track ID"}
	}

	// Scan track
	var track Track
	trackRow := db.DB.QueryRow("SELECT id,type,name FROM tracks WHERE id = $1", trackID)
	trackErr := trackRow.Scan(&track.ID, &track.Type, &track.Name)
	if trackErr == sql.ErrNoRows {
		return rest.Result{}
	} else if trackErr != nil {
		return rest.Result{Error: trackErr}
	}
	trackAndStations.ID = track.ID
	trackAndStations.Type = track.Type
	trackAndStations.Name = track.Name

	// Scan stations
	dbResult := db.SelectMany(&trackAndStations.Stations, "stations",
		"track", "=", track.ID,
		"status", "!=", StationStatusTerminated,
	)
	if dbResult.IsFailed() {
		return rest.Result{Error: dbResult.Error}
	}

	// Hide station credentials
	for _, station := range trackAndStations.Stations {
		station.Credentials = ""
	}

	return rest.Result{}
}

// Get creates a a big mess of data which is perfect for the current frontend because we may not have time to improve it.
func (t4 *StationTasksTests) Get(request *rest.Request) rest.Result {
	trackID, trackIDExists := request.PathArgs["track_id"]
	if !trackIDExists || trackID == "" {
		return rest.Result{Code: 400, Message: "missing track ID"}
	}
	stationShortname, stationShortnameExists := request.PathArgs["station_shortname"]
	if !stationShortnameExists || stationShortname == "" {
		return rest.Result{Code: 400, Message: "missing station shortname"}
	}

	// Scan track
	var track Track
	trackRow := db.DB.QueryRow("SELECT id,type,name FROM tracks WHERE id = $1", trackID)
	trackErr := trackRow.Scan(&track.ID, &track.Type, &track.Name)
	if trackErr == sql.ErrNoRows {
		return rest.Result{}
	}
	if trackErr != nil {
		return rest.Result{Error: trackErr}
	}

	// Scan tasks
	tasks := make([]Task, 0)
	tasksRows, tasksQueryErr := db.DB.Query("SELECT id,track,shortname,name,description,sequence FROM tasks WHERE track = $1 ORDER BY sequence ASC", trackID)
	if tasksQueryErr != nil {
		return rest.Result{Error: tasksQueryErr}
	}
	defer func() {
		tasksRows.Close()
	}()
	for tasksRows.Next() {
		var task Task
		rowErr := tasksRows.Scan(&task.ID, &task.TrackID, &task.Shortname, &task.Name, &task.Description, &task.Sequence)
		if rowErr != nil {
			return rest.Result{Error: rowErr}
		}
		tasks = append(tasks, task)
	}

	// Scan tests
	tests := make([]Test, 0)
	testsRows, testsQueryErr := db.DB.Query("SELECT id,track,task_shortname,shortname,station_shortname,timeslot,name,description,sequence,timestamp,status_success,status_description FROM tests WHERE track = $1 AND station_shortname = $2 AND timeslot = '' ORDER BY sequence ASC",
		trackID, stationShortname)
	if testsQueryErr != nil {
		return rest.Result{Error: testsQueryErr}
	}
	defer func() {
		testsRows.Close()
	}()
	for testsRows.Next() {
		var test Test
		rowErr := testsRows.Scan(&test.ID, &test.TrackID, &test.TaskShortname, &test.Shortname, &test.StationShortname, &test.TimeslotID, &test.Name, &test.Description, &test.Sequence, &test.Timestamp, &test.StatusSuccess, &test.StatusDescription)
		if rowErr != nil {
			return rest.Result{Error: rowErr}
		}
		tests = append(tests, test)
	}

	// Build it
	t4.ID = track.ID
	t4.Type = track.Type
	t4.Name = track.Name
	t4.StationShortname = stationShortname
	t4.Tasks = make([]*stationTasksTestsTask, 0)
	t4TaskMap := make(map[string]*stationTasksTestsTask)
	for _, task := range tasks {
		var t4Task stationTasksTestsTask
		t4Task.ID = task.ID
		t4Task.Shortname = task.Shortname
		t4Task.Name = task.Name
		t4Task.Description = task.Description
		t4Task.Sequence = task.Sequence
		t4Task.Tests = make([]Test, 0)
		t4.Tasks = append(t4.Tasks, &t4Task)
		t4TaskMap[task.Shortname] = &t4Task
	}
	for _, test := range tests {
		t4Task, t4TaskOk := t4TaskMap[test.TaskShortname]
		if !t4TaskOk {
			continue
		}
		t4Task.Tests = append(t4Task.Tests, test)
	}

	return rest.Result{}
}
