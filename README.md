# Tech:Online Backend

Main repo: [Tech:Online](https://github.com/gathering/tech-online)

Forked from the [Gondul API](https://github.com/gathering/gondulapi) repo for Tech:Online 2020. The 2020 version by Kristian Lyngstøl can be found in the [Tech:Online](https://github.com/gathering/tech-online) repo history. The 2021 and 2022 versions can be found here.

## Description

See [Gondul API](https://github.com/gathering/gondulapi) for more details on the underlying framework.

## Development

### Spin Up Local Keycloak Server

Optional, used to test OpenID Connect (or OAuth 2.0) authentication.

1.

### Setup and Run App with Docker Compose

You don't have to use Docker or Docker Compose, but it makes it easier.

1. (First time) Create local config: `cp dev/config.json dev/config.local.json`
1. (First time) Start the DB (detatched): `docker-compose -f dev/docker-compose.yml up -d db`
1. (First time) Apply schema to DB: `dev/prepare-db.sh`
1. Build and start everything: `docker-compose -f dev/docker-compose.yml up --build [-d]`
1. Seed example data: `dev/seed.sh`
1. Profit.

### Devemopment Miscellanea

- Check linting errors: `golint ./...`

## Miscellanea

- This does not feature any kind of automatic DB migration, so you need to manually migrate when upgrading with an existing database (re-applying the schema file for new tables and manually editing existing tables).

## Changes (2022)

Mainly so frontend-people and such can see what changed. This is not a changelog. Also, this repo is not versioned. Yet. lol.

- Rename Go module from `github.com/gathering/gondulapi` to `github.com/gathering/tech-online-backend` (using import alias `techo`).
- Remove temporary, custom endpoints (`/custom/track-stations/` and `/custom/station-tasks-tests/`).
- Changed config structure.

## TODO

### General

- Test new error.
- Remove global state.
- Add docs comment to all packages, with consistent formatting.
- Avoid weird dependencies between packages.
- Cleanup receiver after partial refactoring and general lack of comments.
- Bump Go and repo versions.
- Update file header license.
- Implement OpenID Connect or OAuth 2.0.
- Cleanup admin-by-path stuff and associated "ForAdmin" stuff where admin stuff was on separate endpoints.
- From "database_string" to actual parameters.
- Add "REST" prefix to all Get/Put/Post/Delete/Update.

### Desirable Changes from 2021

- UUIDs are nice but not so nice to remember for manual API calls. Maybe find a way to support both UUIDs and composite keys (e.g. track ID + station shortname) in a clean way?
- Make sure the `print-*.sh` scripts aren't needed.
- Better authn/authz! OAuth2 and app tokens and stuff. No more separate "admin" endpoints. See the OAuth something branch. Not implemented this year because of limited time and considerable frontend changes required.
- Split station state "active" into "ready" and "in-use" or something and move timeslot binding to timeslot.
- Get rid of the temporary "custom" endpoints.
- The DB-layer Select() is nice for dynamic "where"s but makes joins, sorting, limiting etc. kinda impossible. Maybe split out the build-where part and allow using it in manual SQL queries?
- Key-value set of variables for each station (e.g. IP addresses for use in docs templating).
- Endpoints with PATCH semantics, e.g. for easily changing station attributes like state and credentials. Requires changes to gondulapi in order to support the PATCH method and for the DB layer to support both PUT and PATCH semantics.
