//go:build windows && arm64

package pushbin

import _ "embed"

//go:embed push_images_windows_arm64.bin
var embeddedPushBinary []byte
