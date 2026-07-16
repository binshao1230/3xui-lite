package version

// Version is the panel version (semver-ish, without leading "v").
// Must match the GitHub Release tag style: v{Version}
const Version = "0.3.0"

// Repo is the GitHub repository for online updates.
const Repo = "binshao1230/3xui-lite"

// Full returns "v0.3.0" style tag.
func Full() string {
	return "v" + Version
}
