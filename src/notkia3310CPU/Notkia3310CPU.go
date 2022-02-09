package notkia3310CPU

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"math/rand"
	"os"
)

type stackNode struct {
	next  *stackNode
	value byte
}

type stack struct {
	top           *stackNode
	currentLength int
	maxLength     int
}

func (s *stack) push(value byte) error {
	if s.currentLength >= s.maxLength {
		return errors.New("attempted to push on full stack")
	}
	newStackNode := new(stackNode)
	newStackNode.value = value
	newStackNode.next = s.top
	s.currentLength++
	s.top = newStackNode
	return nil
}

func (s *stack) pop() byte {
	if s.top == nil {
		return 0
	}
	v := s.top.value
	s.top = s.top.next
	s.currentLength--
	return v
}

func (s *stack) peek() byte {
	if s.top != nil {
		return s.top.value
	}
	return 0
}

type position struct {
	x byte
	y byte
}

func newPosition(x, y byte) position {
	var p position
	p.x = x
	p.y = y
	return p
}

type N3310 struct {
	FrameBuffer   [84 * 48]byte     // Graphics Array
	memory        [256 * 128]byte   // 32Kb of 2D Memory
	stack         stack             // Memory stack
	gamePad       uint16            // Only 12 buttons, the first nibble is ignored
	instruction   byte              // Befunge Instruction
	pos           position          // Position in the 2D memory
	direction     byte              // Current moving direction
	soundPitch    byte              // Pitch to be played by the audio
	addressLabels map[byte]position // Stores the positions of the labels in memory
	redraw        bool              // Should the screen be redrawn this cycle
	shutdown      bool
}

type instruction func(n *N3310) error

var instructionMap = map[byte]instruction{
	' ': func(n *N3310) error { return nil },
	'@': func(n *N3310) error {
		n.pos.x = 0
		n.pos.y = 0
		n.shutdown = true
		return nil
	},
	// Direction Switch Functions
	'>': func(n *N3310) error {
		n.direction = 0
		return nil
	},
	'<': func(n *N3310) error {
		n.direction = 1
		return nil
	},
	'v': func(n *N3310) error {
		n.direction = 2
		return nil
	},
	'^': func(n *N3310) error {
		n.direction = 3
		return nil
	},
	'?': func(n *N3310) error {
		n.direction = byte(rand.Int() % 4)
		return nil
	},
	// Basic Math Functions
	'+': func(n *N3310) error {
		a, b := n.stack.pop(), n.stack.pop()
		n.stack.push(a + b)
		return nil
	},
	'-': func(n *N3310) error {
		a, b := n.stack.pop(), n.stack.pop()
		n.stack.push(a - b)
		return nil
	},
	'*': func(n *N3310) error {
		a, b := n.stack.pop(), n.stack.pop()
		n.stack.push(a * b)
		return nil
	},
	'/': func(n *N3310) error {
		a, b := n.stack.pop(), n.stack.pop()
		n.stack.push(a / b)
		return nil
	},
	'%': func(n *N3310) error {
		a, b := n.stack.pop(), n.stack.pop()
		n.stack.push(a % b)
		return nil
	},
	// Logic Functions
	'!': func(n *N3310) error {
		if n.stack.pop() == 0 {
			n.stack.push(1)
			return nil
		}
		n.stack.push(0)
		return nil
	},
	'`': func(n *N3310) error {
		a, b := n.stack.pop(), n.stack.pop()
		if b > a {
			n.stack.push(1)
			return nil
		}
		n.stack.push(0)
		return nil
	},
	'|': func(n *N3310) error {
		a := n.stack.pop()
		if a == 0 {
			n.direction = 2
			return nil
		}
		n.direction = 3
		return nil
	},
	'\\': func(n *N3310) error {
		a, b := n.stack.pop(), n.stack.pop()
		n.stack.push(a)
		n.stack.push(b)
		return nil
	},
	// Stack Functions
	':': func(n *N3310) error {
		n.stack.push(n.stack.peek())
		return nil
	},
	'$': func(n *N3310) error {
		n.stack.pop()
		return nil
	},
	'_': func(n *N3310) error {
		a := n.stack.pop()
		if a == 0 {
			n.direction = 0
			return nil
		}
		n.direction = 1
		return nil
	},

	// Memory Functions
	'g': func(n *N3310) error {
		x, y := int(n.stack.pop()), int(n.stack.pop())
		c := n.memory[indexFromPosition(x, y, 256, 128)]
		fmt.Println("Getting ", c, " from ", x, y)
		n.stack.push(c)
		return nil
	},
	'G': func(n *N3310) error {
		x, y := int(n.stack.pop()), int(n.stack.pop())
		c := n.memory[indexFromPosition(x, y, 256, 128)]
		c = hexToByte(c)
		fmt.Println("Getting ", c, " from ", x, y)
		n.stack.push(c)
		return nil
	},
	'p': func(n *N3310) error {
		x, y, v := int(n.stack.pop()), int(n.stack.pop()), n.stack.pop()
		n.memory[indexFromPosition(x, y, 256, 128)] = v
		return nil
	},
	'P': func(n *N3310) error {
		x, y, v := int(n.stack.pop()), int(n.stack.pop()), n.stack.pop()
		n.memory[indexFromPosition(x, y, 256, 128)] = hexToByte(v)
		return nil
	},
	'#': func(n *N3310) error {
		n.updatePosition()
		return nil
	},
	'\'': func(n *N3310) error { // New Function: pushes the next character in memory to the stack and then skips it (Replacement for stringmode)
		n.updatePosition()
		n.stack.push(n.memory[indexFromPosition(int(n.pos.x), int(n.pos.y), 256, 128)])
		return nil
	},
	'j': func(n *N3310) error { // New instruction: pops x and y, jumps to position (x,y) in memory
		x, y := n.stack.pop(), n.stack.pop()
		//pos := n.addressLabels[byte(indexFromPosition(int(x), int(y)))]
		n.pos.x = x
		n.pos.y = y
		return nil
	},
	'l': func(n *N3310) error {
		c := n.stack.pop()
		pos, ok := n.addressLabels[c]
		if !ok {
			return nil
		}
		n.stack.push(pos.y)

		n.stack.push(pos.x + 1)
		//n.pos.x = pos.x
		//n.pos.y = pos.y
		return nil
	},
	// Drawing Functions
	'.': func(n *N3310) error { // Changed from Befunge: Now Pops X and Y, and then draws a single pixel at the position (X,Y)
		x, y := int(n.stack.pop()), int(n.stack.pop())
		n.FrameBuffer[indexFromPosition(x, y, 84, 48)] = 1
		n.redraw = true
		return nil
	},
	',': func(n *N3310) error { // Changed from Befunge: Now Pops X1, Y1, X2 and Y2, and draws the sprite located at memory(X1,Y1) with in the position (X2,Y2) with transparent colour
		// Sprites are defined as a sequence of 8 bytes, each byte defines one line of the sprite, with a 0 indicating that the pixel should
		// remain as it previously was, and an 1 indicating that the pixel should be of the light colour
		x1, y1, x2, y2 := n.stack.pop(), n.stack.pop(), n.stack.pop(), n.stack.pop()
		var currentSpriteByte byte
		curentSpritePos := 0
		lines := make([]byte, 8)
		for i := 0; i < 16; i += 2 {
			currentSpriteByte = n.memory[indexFromPosition(int(x1)+i, int(y1), 256, 128)]
			var newLine byte
			newLine = hexToByte(currentSpriteByte) << 4
			curentSpritePos++
			currentSpriteByte = n.memory[indexFromPosition(int(x1)+i+1, int(y1), 256, 128)]
			newLine |= hexToByte(currentSpriteByte)
			//curentSpritePos++
			lines[i/2] = newLine
		}
		for i, v := range lines {
			for x := 0; x < 8; x++ {
				drawPos := indexFromPosition((x + int(x2)), i+int(y2), 84, 48)
				if v&(128>>x) == 0 {
					n.FrameBuffer[drawPos] = 0
					//fmt.Printf("0")
				} else {
					n.FrameBuffer[drawPos] = 1
				}
			}
		}
		n.redraw = true

		return nil
	},
	'C': func(n *N3310) error { // New Instrucion: Clear Screen
		for i := 0; i < 84*48; i++ {
			n.FrameBuffer[i] = 0
		}
		n.redraw = true

		return nil
	},
}

func indexFromPosition(x, y, max_x, max_y int) int {
	x = x % max_x
	y = y % max_y
	return (int(y) * max_x) + int(x)
}

func (n *N3310) updatePosition() {
	switch n.direction {
	case 0:
		n.pos.x++
	case 1:
		n.pos.x--
	case 2:
		n.pos.y++
	case 3:
		n.pos.y--
	}
}

func (n *N3310) RunCycle() {
	index := indexFromPosition(int(n.pos.x), int(n.pos.y), 256, 128)
	//fmt.Println(n.stack)
	inst, ok := instructionMap[n.memory[index]]
	if !ok {
		b := hexToByte(n.memory[index])
		if b != 0xFF {
			n.stack.push(b)
		}
	} else {
		inst(n)
	}
	n.updatePosition()
}

func hexToByte(c byte) byte {
	if c >= '0' && c <= '9' {
		return c - '0'
	}
	if c >= 'a' && c <= 'f' {
		return c - 'a' + 10
	}
	return 0xFF
}

func (n *N3310) loadInLabels() {
	// CLEARS OUT OLD LABELS
	for k := range n.addressLabels {
		delete(n.addressLabels, k)
	}
	for yPos := 0; yPos < 128; yPos++ {
		for xPos := 0; xPos < 255; xPos++ {
			index := indexFromPosition(xPos, yPos, 256, 128)
			labelIndex := indexFromPosition(xPos+1, yPos, 256, 128)
			if n.memory[index] == ';' {
				c := n.memory[labelIndex]
				n.addressLabels[c] = newPosition(byte(xPos+1), byte(yPos))
			}
		}
	}
}

func (n *N3310) InitializeNotkia() {
	n.gamePad = 0
	n.instruction = 0
	n.direction = 0
	n.soundPitch = 0
	n.stack.top = nil
	n.stack.currentLength = 0
	n.stack.maxLength = 512
	n.pos.x = 0
	n.pos.y = 0
	for i := 0; i < 84*48; i++ {
		n.FrameBuffer[i] = 0
	}
	for i := 0; i < 128*64; i++ {
		n.memory[i] = 0
	}
	n.addressLabels = make(map[byte]position)
}

func (n *N3310) ReadCode(romName string) {
	rom, err := os.Open(romName)
	if err != nil {
		panic(err)
	}
	reader := bufio.NewReader(rom)
	buf := make([]byte, 1)
	i := 0
	currentPosInLine := 0
	for {
		_, err := reader.Read(buf)
		if err != nil && !errors.Is(err, io.EOF) {
			panic(err)
		}
		b := buf[0]
		currentPosInLine++
		if b == '\n' {
			for j := 0; j < 256-currentPosInLine; j++ {
				n.memory[i+j] = 0
				i++
			}
			currentPosInLine = 0
		} else if currentPosInLine == 255 {
			currentPosInLine = 0
		}
		//fmt.Printf("%c", b)
		n.memory[i] = b
		i++
		if err != nil {
			// end of file
			break
		}
	}
	n.loadInLabels()
}

func (n *N3310) TextDraw() {
	for y := 0; y < 48; y++ {
		for x := 0; x < 84; x++ {
			if n.FrameBuffer[indexFromPosition(x, y, 84, 48)] == 0 {
				fmt.Printf("_")
			} else {
				fmt.Printf("O")
			}
		}
		fmt.Println()
	}
}

/*func main() {
	var n N3310
	n.InitializeNotkia()

	n.ReadCode("code")
	n.loadInLabels()
	fmt.Println()
	for {
		for !n.shutdown {
			//for i := 0; i < 20; i++ {
			n.RunCycle()
			n.textDraw()
			n.pos = newPosition(0, 0)
			//fmt.Println(n.stack)
		}
		n.textDraw()
		s, _ := time.ParseDuration("16ms")
		time.Sleep(s)

	}
}*/
