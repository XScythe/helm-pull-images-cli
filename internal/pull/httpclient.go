package pull

import (
	"net/http"
	"time"
)

var outboundHTTPClient = &http.Client{
	Timeout: 30 * time.Second,
}
