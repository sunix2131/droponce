package qr

import qrcode "github.com/skip2/go-qrcode"

func PNG(value string) ([]byte, error) {
	return qrcode.Encode(value, qrcode.Medium, 512)
}
