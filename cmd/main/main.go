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

package main

import (
	"github.com/gathering/tech-online-backend/config"
	"github.com/gathering/tech-online-backend/db"
	"github.com/gathering/tech-online-backend/receiver"
	log "github.com/sirupsen/logrus"
)

func main() {
	if err := config.ParseConfig("config.json"); err != nil {
		log.WithError(err).Fatal("Failed to read config file")
		return
	}
	log.Info("Read config file")
	if err := db.Connect(config.Config.DatabaseString); err != nil {
		log.WithError(err).Fatal("Failed to read config file")
		return
	}
	log.Info("Connected to database")
	receiver.Start()
}