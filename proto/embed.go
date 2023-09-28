package proto

import (
	// this is for embedding plane.swagger.json
	_ "embed"
)

// SwaggerJSON is a file plane.swagger.json
//
//go:embed plane.swagger.json
var SwaggerJSON []byte
