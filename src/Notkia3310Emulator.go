package main

import (
	"fmt"
	nk "notkiaCPU"
	"os"
	"time"

	"github.com/veandco/go-sdl2/sdl"
)

const darkR = 0x43
const darkG = 0x52
const darkB = 0x3d
const lightR = 0xc7
const lightG = 0xf0
const lightB = 0xd8

func main() {

	fmt.Println("Initializing NK33: Notkia-3310 Emulator")
	fmt.Println("...")
	var n nk.N3310

	n.InitializeNotkia()
	fmt.Println("Initialization succesful")
	args := os.Args[1:]
	romName := args[0]
	fmt.Println("Loading ROM: ", romName)
	fmt.Println("...")
	n.ReadCode(romName)
	fmt.Println("Loading succesful")
	window, renderer, err := sdl.CreateWindowAndRenderer(840, 480, sdl.WINDOW_BORDERLESS|sdl.WINDOW_RESIZABLE)
	renderer.SetLogicalSize(84, 48)
	if err != nil {
		return
	}
	for {
		for i := 0; i < 60; i++ {
			n.RunCycle()
			updateGraphics(window, renderer, n.FrameBuffer)
		}
		time.Sleep(time.Millisecond * 16)
	}
}

func updateGraphics(window *sdl.Window, renderer *sdl.Renderer, frameBuffer [84 * 48]byte) {
	for y := 0; y < 48; y++ {
		for x := 0; x < 84; x++ {
			if frameBuffer[(y*84)+x] == 0 {
				renderer.SetDrawColor(darkR, darkG, darkB, 255)
			} else {
				renderer.SetDrawColor(lightR, lightG, lightB, 255)
			}
			renderer.DrawPoint(int32(x), int32(y))
		}
	}
	renderer.Present()
}
