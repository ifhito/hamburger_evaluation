// Package repository implements the repository interfaces declared in
// usecase, using sqlc-generated code (sqlcgen) over pgx.
//
// sqlc output lives in the sqlcgen subpackage and is never edited by hand;
// change db/queries/ and regenerate instead. Populated in later stories.
package repository
