package web

import "embed"

// Files contains the dependency-free browser client served by the API process.
//
//go:embed static/*
var Files embed.FS
