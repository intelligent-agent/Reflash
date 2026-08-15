package main

import (
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"log"
	"os"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/golang/freetype"
	"github.com/golang/freetype/truetype"
	"golang.org/x/image/font"
	"golang.org/x/image/math/fixed"
)

const (
	framebufferDevice = "/dev/fb0"
	bitsPerPixel      = 32
)

const (
	PORTRAIT  = iota
	LANDSCAPE = iota
)

var fb_width int
var fb_height int
var fb_max int
var fb_min int

var blue color.RGBA
var white color.RGBA
var black color.RGBA

var img *image.RGBA
var fbMem []byte
var fb *os.File

var reDraw bool
var orientation int

// End the kernel's deferred fbcon takeover, and wait for it to finish.
//
// CONFIG_FRAMEBUFFER_CONSOLE_DEFERRED_TAKEOVER holds fbcon back so the
// bootloader's splash stays on screen until something actually wants the
// display. It is released by the first console output on the VT - and Reflash
// boots with console=serial, so that never happens on its own.
//
// It matters because the panel is driven by simpledrm off the framebuffer
// u-boot handed over, and simpledrm only copies its buffer out to the scanout
// memory during an atomic commit. Until some client enables the CRTC there is
// no commit, so everything drawn here lands in the buffer and never reaches the
// glass: the panel just keeps showing u-boot's pixels. sun4i-drm used to hide
// this by doing an initial modeset at probe time.
//
// The release is driven by the dummy console's output notifier, so it takes
// real printable output to fire: measured on hardware, "reflash\n" triggers it
// while a lone "\n" or a single space does not - those are handled by the vt
// layer without ever reaching the dummy console's putcs. Hence a proper word
// here rather than one byte. The text itself is never seen: it goes to the
// dummy console, and fbcon clears the framebuffer as it takes over.
//
// The takeover is asynchronous and that clear would wipe anything already
// drawn, so wait for it to land before returning.
func endDeferredConsoleTakeover() {
	tty, err := os.OpenFile("/dev/tty1", os.O_WRONLY, 0)
	if err != nil {
		// Headless, or no VT - nothing to take over.
		fmt.Printf("Could not open /dev/tty1 to start console takeover: %v\n", err)
		return
	}
	// Trailing clear-screen + cursor-home in case fbcon was already active and
	// the word would otherwise be left on the panel under Reflash's drawing.
	tty.Write([]byte("reflash\n\033[2J\033[H"))
	tty.Close()

	for i := 0; i < 100; i++ {
		if consoleTakenOver() {
			fmt.Println("Console takeover complete")
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	fmt.Println("Timed out waiting for console takeover; drawing anyway")
}

// True once a framebuffer console is bound, which is what tells us the CRTC has
// been enabled and simpledrm is scanning out.
func consoleTakenOver() bool {
	entries, err := os.ReadDir("/sys/class/vtconsole")
	if err != nil {
		return false
	}
	for _, e := range entries {
		base := "/sys/class/vtconsole/" + e.Name()
		name, err := os.ReadFile(base + "/name")
		if err != nil || !strings.Contains(string(name), "frame buffer device") {
			continue
		}
		bound, err := os.ReadFile(base + "/bind")
		if err == nil && strings.TrimSpace(string(bound)) == "1" {
			return true
		}
	}
	return false
}

func ScreenInit() {
	endDeferredConsoleTakeover()

	content, err := os.ReadFile("/sys/class/graphics/fb0/virtual_size")
	if err != nil {
		fmt.Printf("Error opening /sys/class/graphics/fb0/virtual_size: %v\n", err)
		return
	}
	sizes := strings.Split(strings.TrimSpace(string(content)), ",")
	fb_width, _ = strconv.Atoi(sizes[0])
	fb_height, _ = strconv.Atoi(sizes[1])
	fb_min = min(fb_width, fb_height)
	fb_max = max(fb_width, fb_height)

	fmt.Println("Screen width: ", fb_width)
	fmt.Println("Screen height: ", fb_height)
	fmt.Println("Screen min: ", fb_min)
	fmt.Println("Screen max: ", fb_max)
	if fb_width > fb_height {
		orientation = LANDSCAPE
	} else {
		orientation = PORTRAIT
	}
	blue = color.RGBA{4, 163, 229, 255}
	white = color.RGBA{201, 201, 201, 255}
	black = color.RGBA{41, 42, 44, 255}

	fb, err = os.OpenFile(framebufferDevice, os.O_RDWR, 0)
	if err != nil {
		fmt.Printf("Error opening framebuffer device: %v\n", err)
		return
	}

	size := fb_width * fb_height * bitsPerPixel / 8
	fbMem, err = syscall.Mmap(int(fb.Fd()), 0, int(size), syscall.PROT_READ|syscall.PROT_WRITE, syscall.MAP_SHARED)
	if err != nil {
		fmt.Printf("Error mapping framebuffer to memory: %v\n", err)
		return
	}

	img = image.NewRGBA(image.Rect(0, 0, fb_min, fb_min))
	ips := make([]string, 0)
	Draw(0, "IDLE", 0, ips, "")
}

func ScreenClose() {
	fb.Close()
	syscall.Munmap(fbMem)
}

// TODO: This is horrobly inefficient and should be optimized.
func Draw(progress float32, state string, rot int, ips []string, version string) {
	// No framebuffer mapped → ScreenInit either failed (headless) or was
	// never called (tests). Skip rendering entirely; updateDisplay still
	// keeps its bookkeeping in sync.
	if fbMem == nil {
		return
	}
	img = image.NewRGBA(image.Rect(0, 0, fb_min, fb_min))
	clear(img)

	if isRebootArmed() {
		// A flash finished; prompt the user to pull the USB drive (the board
		// reboots into the new image on removal). Keyed off the arm flag, not
		// the state, because getProgress flips FINISHED->IDLE on the first poll.
		drawLogo(img, (fb_min/2)-95)
		drawText(img, "REFLASH", 50, (fb_min/2)+55)
		drawText(img, "Flash complete", 28, (fb_min/2)+110)
		drawText(img, "Remove USB drive", 28, (fb_min/2)+145)
	} else if state == "IDLE" {
		drawLogo(img, (fb_min/2)-95)
		drawText(img, "REFLASH", 50, (fb_min/2)+55)
		for i, s := range ips {
			drawText(img, s, 20, (fb_min/2)+125+(20*i))
		}
		// The screen is sometimes the only information available (e.g. no
		// network reachable yet) - show the running version so it's visible
		// without needing the web UI.
		if version != "" {
			drawText(img, version, 16, (fb_min/2)+125+(20*len(ips))+20)
		}
	} else {
		if fb_min > 700 {
			drawLogo(img, (fb_min/2)-250)
			drawText(img, "REFLASH", 50, (fb_min/2)-100)
			drawProgressBar(img, (fb_min / 2), progress)
			drawText(img, state, 30, (fb_min/2)+120)
		} else {
			drawLogo(img, (fb_min/2)-210)
			drawText(img, "REFLASH", 50, (fb_min/2)-(110-50))
			drawProgressBar(img, (fb_min / 2), progress)
			drawText(img, state, 30, (fb_min/2)+60+36)
		}
	}

	if rot == 90 {
		img = rotate90Degrees(img)
	} else if rot == 180 {
		img = rotate90Degrees(img)
		img = rotate90Degrees(img)
	} else if rot == 270 {
		img = rotate90Degrees(img)
		img = rotate90Degrees(img)
		img = rotate90Degrees(img)
	}
	if orientation == PORTRAIT {
		img = translateImage(img, 0, (fb_max/2)-(fb_min/2))
	} else {
		img = translateImage(img, (fb_max/2)-(fb_min/2), 0)
	}

	if fbMem != nil {
		copyImageToFramebuffer(img, fbMem)
	}
}

func clear(img *image.RGBA) {
	bg := image.Black
	draw.Draw(img, img.Bounds(), bg, image.Point{}, draw.Src)
}

func drawText(img *image.RGBA, text string, size float64, y int) {
	fontBytes, err := os.ReadFile("/usr/local/share/fonts/Roboto-Light.ttf")
	if err != nil {
		log.Println(err)
		return
	}
	f, err := freetype.ParseFont(fontBytes)
	if err != nil {
		log.Println(err)
		return
	}

	fg := image.White
	c := freetype.NewContext()
	c.SetDPI(72)
	c.SetFont(f)
	c.SetFontSize(size)
	c.SetClip(img.Bounds())
	c.SetDst(img)
	c.SetSrc(fg)
	c.SetHinting(font.HintingNone)

	d := &font.Drawer{
		Dst: img,
		Src: fg,
		Face: truetype.NewFace(f, &truetype.Options{
			Size:    size,
			DPI:     72,
			Hinting: font.HintingNone,
		}),
	}
	d.Dot = fixed.Point26_6{
		X: (fixed.I(fb_min) - d.MeasureString(text)) / 2,
		Y: fixed.I(y),
	}
	d.DrawString(text)
}

func drawLine(img *image.RGBA, x0, y0, x1, y1 int, col color.Color) {
	// Bresenham's line algorithm
	dx := abs(x1 - x0)
	dy := abs(y1 - y0)
	sx := 1
	sy := 1
	if x0 > x1 {
		sx = -1
	}
	if y0 > y1 {
		sy = -1
	}

	err := dx - dy

	for {
		img.Set(x0, y0, col)
		if x0 == x1 && y0 == y1 {
			break
		}

		e2 := 2 * err
		if e2 > -dy {
			err -= dy
			x0 += sx
		}
		if e2 < dx {
			err += dx
			y0 += sy
		}
	}
}

func drawRect(img *image.RGBA, x int, y int, w int, h int) {
	drawLine(img, x, y, x+w, y, white)     // horizontal 1
	drawLine(img, x, y+h, x+w, y+h, white) // horzontal 2
	drawLine(img, x, y, x, y+h, white)     // vertical 1
	drawLine(img, x+w, y, x+w, y+h, white) // vertical 2
}

func drawLogo(img *image.RGBA, y int) {
	s := 40
	o := 15
	x := (fb_min / 2) - 20
	drawRect(img, x, y, s, s)

	drawLine(img, x, y, x+o, y+o, white)
	drawLine(img, x+s, y, x+o+s, y+o, white)
	drawLine(img, x, y+s, x+o, y+s+o, white)
	drawLine(img, x+s, y+s, x+o+s, y+s+o, white)

	y += o
	x += o
	drawRect(img, x, y, s, s)
}

func drawProgressBar(img *image.RGBA, y int, progress float32) {
	var pad float32 = float32(fb_min / 4)
	var pWidth float32 = float32(fb_min) - (2 * pad)
	var start_y int = y - 18
	var stop_y int = y + 18
	var start_x int = int(pad)
	var stop_x int = int(pad + pWidth*progress)
	m := 6

	drawRect(img, start_x-m, start_y-m, int(pWidth)+(2*m), 36+(2*m))
	for y := start_y; y < stop_y; y++ {
		for x := start_x; x < stop_x; x++ {
			img.Set(x, y, blue)
		}
	}
}

func copyImageToFramebuffer(img *image.RGBA, fbMem []byte) {
	bytesPerPixel := bitsPerPixel / 8
	for y := 0; y < fb_height; y++ {
		for x := 0; x < fb_width; x++ {
			offset := (y*fb_width + x) * bytesPerPixel
			c := img.At(x, y).(color.RGBA)
			fbMem[offset] = c.B
			fbMem[offset+1] = c.G
			fbMem[offset+2] = c.R
			fbMem[offset+3] = c.A
		}
	}
}

func rotate90Degrees(img *image.RGBA) *image.RGBA {
	rotatedImg := image.NewRGBA(image.Rect(0, 0, img.Bounds().Dy(), img.Bounds().Dx()))
	for x := 0; x < rotatedImg.Bounds().Dx(); x++ {
		for y := 0; y < rotatedImg.Bounds().Dy(); y++ {
			rotatedImg.Set(x, y, img.At(y, img.Bounds().Dx()-1-x))
		}
	}

	return rotatedImg
}

func translateImage(img *image.RGBA, dx int, dy int) *image.RGBA {
	sizeX := img.Bounds().Dx()
	sizeY := img.Bounds().Dy()
	translatedImg := image.NewRGBA(image.Rect(0, 0, fb_width, fb_height))

	for x := 0; x < sizeX; x++ {
		for y := 0; y < sizeY; y++ {
			translatedX := (x + dx)
			translatedY := (y + dy)
			translatedImg.Set(translatedX, translatedY, img.At(x, y))
		}
	}

	return translatedImg
}

func abs(x int) int {
	if x < 0 {
		return -x
	}
	return x
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
