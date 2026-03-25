package profile

import (
	"io"

	"github.com/amadeusitgroup/cds/internal/clog"
)

type profileSrcOptions struct {
	profileReader io.Reader
}

type profileOutputOptions struct {
	cacheWriter io.Writer
}

type profile struct {
	profileData any
}

var (
	loadedProfile profile
)

// Option to load profile from reader
func WithReader(r io.Reader) func(*profileSrcOptions) {
	return func(lp *profileSrcOptions) {
		lp.profileReader = r
	}
}

// New profile
func New(opts ...func(*profileSrcOptions)) profile {
	clog.Warn("profile.Init Not Implemented")
	profileSrcOptions := &profileSrcOptions{}
	for _, opt := range opts {
		opt(profileSrcOptions)
	}
	clog.Warn("[profile.Init] Would use following profile", clog.NewLoggable("profile", profileSrcOptions))
	if profileSrcOptions.profileReader != nil {
		loadedProfile = readProfileFromReader(profileSrcOptions.profileReader)
		return loadedProfile
	}
	clog.Warn("No profile source provided")
	return profile{}
}

func readProfileFromReader(r io.Reader) profile {
	// Read the profile from the reader
	clog.Warn("profile.readProfileFromReader Not Implemented")
	clog.Warn("Would read profile from reader", clog.NewLoggable("reader", r))
	return profile{}
}

func WithWriter(w io.Writer) func(*profileOutputOptions) {
	return func(options *profileOutputOptions) {
		options.cacheWriter = w
	}
}

// Used to modify current profile (cds space profile init <path>)
// Saves the loaded profile to the given path
func (p profile) Save(opts ...func(*profileOutputOptions)) {
	if p.profileData == nil {
		clog.Warn("No profile data to save")
		return
	}
	clog.Warn("profile.Save Not Implemented")
	options := &profileOutputOptions{}
	for _, opt := range opts {
		opt(options)
	}
	if options.cacheWriter != nil {
		p.saveUsingWriter(options.cacheWriter)
		return
	}
	clog.Warn("No saving method provided")
}

// Save profile using writer
func (p profile) saveUsingWriter(w io.Writer) {
	clog.Warn("profile.saveUsingWriter Not Implemented")
	clog.Warn("Would save profile using writer", clog.NewLoggable("writer", w), clog.NewLoggable("profile data p", p.profileData))
}

// Merge the profile data with the devcontainer profile
func Merge(devcontainerProfile any) any {
	clog.Warn("profile.Merge Not Implemented returning devcontainer profile")
	clog.Warn("Would Merge", clog.NewLoggable("profile data", loadedProfile.profileData), clog.NewLoggable("devcontainer profile", devcontainerProfile))
	return devcontainerProfile
}
