package main

import "embed"

// assets contains the dashboard templates and static files bundled into the
// portal binary.
//
//go:embed templates/*.html static/*
var assets embed.FS
