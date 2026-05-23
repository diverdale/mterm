// Package appmeta holds app-wide identity constants. Centralized so a future
// rename is a one-edit affair instead of a sed-sweep.
package appmeta

// Name is the display label shown in chrome (title bar, dialogs, warnings).
const Name = "mterm"

// DirName is the subdirectory used under XDG paths (~/.config/<DirName>/,
// future log/cache dirs). Often equal to Name, but kept separate so a brand
// rename doesn't force users to migrate existing config directories.
const DirName = "mterm"
