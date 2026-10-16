package assets

import "embed"

//go:embed frontend/dist migrations/*.up.sql migrations/postgres/*.up.sql schemas/import.schema.json
var FS embed.FS
