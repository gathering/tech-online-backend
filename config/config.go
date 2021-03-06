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

package config

import (
	"encoding/json"
	"io/ioutil"

	"github.com/google/uuid"
	log "github.com/sirupsen/logrus"
)

// Config covers global configuration, and if need be it will provide
// mechanisms for local overrides (similar to Skogul).
var Config struct {
	ListenAddress  string                               `json:"listen_address"`  // Defaults to :8080
	DatabaseString string                               `json:"database_string"` // For database connections
	SitePrefix     string                               `json:"site_prefix"`     // URL prefix, e.g. "/api"
	Debug          bool                                 `json:"debug"`           // Enables trace-debugging
	OAuth2         OAuth2Config                         `json:"oauth2"`          // OAuth2 section
	Unicorn        UnicornConfig                        `json:"unicorn"`         // Unicorn IdP section
	ServerTracks   map[string]ServerTrackConfig         `json:"server_tracks"`   // Static config for server tracks
	AccessTokens   map[uuid.UUID]AccessTokenEntryConfig `json:"access_tokens"`   // Static config for server tracks
}

// OAuth2Config contains the OAuth2 config
type OAuth2Config struct {
	ClientID     string `json:"client_id"`     // Client ID
	ClientSecret string `json:"client_secret"` // Client Secret
	AuthURL      string `json:"auth_url"`      // Authorize URL
	TokenURL     string `json:"token_url"`     // Token URL
	RedirectURL  string `json:"redirect_url"`  // Redirect URL
}

// UnicornConfig contains the Unicorn IdP config.
type UnicornConfig struct {
	ProfileURL string `json:"profile_url"` // URL to the Unicorn IDP profile endpoint
}

// ServerTrackConfig contains the static config for a single server track.
type ServerTrackConfig struct {
	BaseURL          string `json:"base_url"`
	TaskType         string `json:"task_type"`
	MaxInstancesSoft int    `json:"max_instances_soft"` // Number of instances where participants are allowed to spin up their own
	MaxInstancesHard int    `json:"max_instances_hard"` // Number of instances where operators/admins may spin up another one
	AuthUsername     string `json:"auth_username"`
	AuthPassword     string `json:"auth_password"`
}

// AccessTokenEntryConfig contains the static config for a single non-user access token.
type AccessTokenEntryConfig struct {
	Key     string `json:"key"`
	Role    string `json:"role"`
	Comment string `json:"comment"`
}

// ParseConfig reads a file and parses it as JSON, assuming it will be a
// valid configuration file.
func ParseConfig(file string) error {
	dat, err := ioutil.ReadFile(file)
	if err != nil {
		return err
	}
	if err := json.Unmarshal(dat, &Config); err != nil {
		return err
	}
	if Config.Debug {
		log.SetLevel(log.TraceLevel)
	}
	return nil
}
