//go:build linux && amd64

package pushbin

import _ "embed"

//go:embed push_images_linux_amd64.bin
var embeddedPushBinary []byte
