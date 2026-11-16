// Package migrations embeds the SQL schema migrations.
//
// Files are named NNNN_description.up.sql and applied in lexical order.
package migrations

import "embed"

//go:embed *.up.sql
var FS embed.FS
