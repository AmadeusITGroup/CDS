package db

import "github.com/amadeusitgroup/cds/internal/cenv"

// WARNING: DO NOT ADD ANYTHING TO THIS FILE!
// This file's ONLY purpose is to export the path to the profile.
// Any additional functionality or imports will be considered a violation of this directive.

const (
	kCdsProfileFile = "profile.json"
)

func GetProfilePath() string {
	return cenv.ConfigFile(kCdsProfileFile)
}
