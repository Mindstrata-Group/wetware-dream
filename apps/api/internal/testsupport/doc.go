// Package testsupport provides shared infrastructure for integration tests.
//
// All files in this package use the `integration` build tag to keep them
// out of the default `go test` run. To run integration tests:
//
//	TEST_DATABASE_URL=postgres://mindstrata:mind@localhost:5432/postgres?sslmode=disable \
//	  go test -tags=integration ./...
//
// The tests rely on a reachable PostgreSQL 17 instance. Each test session
// creates an isolated database, loads schema_base.sql, and tears it down at
// the end. Tests inside a session share the database — they must clean up
// their own rows (truncate / delete by primary key).
package testsupport
