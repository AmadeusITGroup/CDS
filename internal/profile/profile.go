package profile

import (
	"io"

	"github.com/amadeusitgroup/cds/internal/clog"
)

type profileSrcOptions struct {
	localpath     string
	profileReader io.Reader
	remoteUrl     string
}

type profileOutputOptions struct {
	cachePath   string
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

// Option to load profile from path
func WithPath(path string) func(*profileSrcOptions) {
	return func(lp *profileSrcOptions) {
		lp.localpath = path
	}
}

// Option to load profile from remote url
func WithUrl(url string) func(*profileSrcOptions) {
	return func(lp *profileSrcOptions) {
		lp.remoteUrl = url
	}
}

// New profile
func New(opts ...func(*profileSrcOptions)) profile {
	// Read the profile from the cached path
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
	if profileSrcOptions.localpath != "" {
		loadedProfile = readFromLocalPath(profileSrcOptions.localpath)
		return loadedProfile
	}
	if profileSrcOptions.remoteUrl != "" {
		loadedProfile = readFromRemoteURL(profileSrcOptions.remoteUrl)
		return loadedProfile
	}
	clog.Warn("No profile source provided")
	return profile{}
}

func readFromLocalPath(path string) profile {
	// Read the profile from the local path
	clog.Warn("profile.readLocalProfile Not Implemented")
	clog.Warn("Would read profile", clog.NewLoggable("path", path))
	return profile{}
}

func readFromRemoteURL(url string) profile {
	// Read the profile from the remote url
	clog.Warn("profile.readRemoteProfile Not Implemented")
	clog.Warn("Would read profile", clog.NewLoggable("url", url))
	return profile{}
}

func readProfileFromReader(r io.Reader) profile {
	// Read the profile from the reader
	clog.Warn("profile.readProfileFromReader Not Implemented")
	clog.Warn("Would read profile from reader", clog.NewLoggable("reader", r))
	return profile{}
}

func WithWriter(w io.Writer) func(*profileOutputOptions) {
	// Prepare the profile from the writer
	return func(options *profileOutputOptions) {
		options.cacheWriter = w
	}
}

func WithOutputPath(path string) func(*profileOutputOptions) {
	// Prepare the profile from the path
	return func(options *profileOutputOptions) {
		options.cachePath = path
	}
}

// Used to modify current profile (cds space profile init <path>)
// Saves the loaded profile to the given path
func (p profile) Save(opts ...func(*profileOutputOptions)) {
	if p.profileData == nil {
		clog.Warn("No profile data to save")
		return
	}
	clog.Warn("profile.Init Not Implemented")
	options := &profileOutputOptions{}
	for _, opt := range opts {
		opt(options)
	}
	if options.cacheWriter != nil {
		p.saveUsingWriter(options.cacheWriter)
		return
	}
	if options.cachePath != "" {
		p.saveUsingLocalPath(options.cachePath)
		return
	}
	clog.Warn("No saving method provided")
}

// Save profile using writer
func (p profile) saveUsingWriter(w io.Writer) {
	// Save the profile using the writer
	clog.Warn("profile.saveUsingWriter Not Implemented")
	clog.Warn("Would save profile using writer", clog.NewLoggable("writer", w), clog.NewLoggable("profile data p", p.profileData))
}

// Save profile using local path
func (p profile) saveUsingLocalPath(path string) {
	// Save the profile using the local path
	clog.Warn("profile.saveUsingLocalPath Not Implemented")
	clog.Warn("Would save profile using path", clog.NewLoggable("path", path), clog.NewLoggable("profile data p", p.profileData))
}

// Merge the profile data with the devcontainer profile
func Merge(devcontainerProfile any) any {
	clog.Warn("profile.Merge Not Implemented returning devcontainer profile")
	clog.Warn("Would Merge", clog.NewLoggable("profile data", loadedProfile.profileData), clog.NewLoggable("devcontainer profile", devcontainerProfile))
	return devcontainerProfile
}
