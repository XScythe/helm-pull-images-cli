//go:build windows && amd64

package pushbin

import _ "embed"

//go:embed push_images_windows_amd64.bin
var embeddedPushBinary []byte
