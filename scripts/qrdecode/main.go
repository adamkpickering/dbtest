// Command qrdecode reads a QR code image and prints its decoded text to stdout.
//
// It exists because zbarimg (the obvious CLI choice) segfaults on some macOS
// homebrew builds of libzbar. gozxing is a pure-Go decoder with no native
// dependencies, so it works the same everywhere our dev tooling runs.
//
// Usage: go run . <image-path>
package main

import (
	"fmt"
	"image"
	_ "image/jpeg"
	_ "image/png"
	"os"

	"github.com/makiuchi-d/gozxing"
	"github.com/makiuchi-d/gozxing/qrcode"
)

func main() {
	if len(os.Args) != 2 {
		fmt.Fprintln(os.Stderr, "usage: qrdecode <image-path>")
		os.Exit(2)
	}

	f, err := os.Open(os.Args[1])
	if err != nil {
		fmt.Fprintln(os.Stderr, "qrdecode:", err)
		os.Exit(1)
	}
	defer f.Close()

	img, _, err := image.Decode(f)
	if err != nil {
		fmt.Fprintln(os.Stderr, "qrdecode: decoding image:", err)
		os.Exit(1)
	}

	bmp, err := gozxing.NewBinaryBitmapFromImage(img)
	if err != nil {
		fmt.Fprintln(os.Stderr, "qrdecode: preparing bitmap:", err)
		os.Exit(1)
	}

	result, err := qrcode.NewQRCodeReader().Decode(bmp, nil)
	if err != nil {
		fmt.Fprintln(os.Stderr, "qrdecode: decoding QR:", err)
		os.Exit(1)
	}

	fmt.Print(result.GetText())
}
