package assets

import "embed"

//go:embed frontend/dist migrations/*.sql
var FS embed.FS
