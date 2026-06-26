//go:build darwin && amd64

package pushbin

import _ "embed"

//go:embed push_images_darwin_amd64.bin
var embeddedPushBinary []byte
