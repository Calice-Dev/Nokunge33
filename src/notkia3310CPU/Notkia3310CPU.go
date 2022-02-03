package main

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
	return s.top.value
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

type n3310 struct {
	frameBuffer   [84 * 48]byte     // Graphics Array
	memory        [256 * 128]byte   // 32Kb of 2D Memory
	stack         stack             // Memory stack
	gamePad       uint16            // Only 12 buttons, the first nibble is ignored
	instruction   byte              // Befunge Instruction
	pos           position          // Position in the 2D memory
	direction     byte              // Current moving direction
	soundPitch    byte              // Pitch to be played by the audio
	addressLabels map[byte]position // Stores the positions of the labels in memory
}

type instruction func(n *n3310) error

var instructionMap = map[byte]instruction{
	' ': func(n *n3310) error { return nil },
	// Direction Switch Functions
	'>': func(n *n3310) error {
		n.direction = 0
		return nil
	},
	'<': func(n *n3310) error {
		n.direction = 1
		return nil
	},
	'v': func(n *n3310) error {
		n.direction = 2
		return nil
	},
	'^': func(n *n3310) error {
		n.direction = 3
		return nil
	},
	'?': func(n *n3310) error {
		n.direction = byte(rand.Int() % 4)
		return nil
	},
	// Basic Math Functions
	'+': func(n *n3310) error {
		a, b := n.stack.pop(), n.stack.pop()
		n.stack.push(a + b)
		return nil
	},
	'-': func(n *n3310) error {
		a, b := n.stack.pop(), n.stack.pop()
		n.stack.push(a - b)
		return nil
	},
	'*': func(n *n3310) error {
		a, b := n.stack.pop(), n.stack.pop()
		n.stack.push(a * b)
		return nil
	},
	'/': func(n *n3310) error {
		a, b := n.stack.pop(), n.stack.pop()
		n.stack.push(a / b)
		return nil
	},
	'%': func(n *n3310) error {
		a, b := n.stack.pop(), n.stack.pop()
		n.stack.push(a % b)
		return nil
	},
	// Logic Functions
	'!': func(n *n3310) error {
		if n.stack.pop() == 0 {
			n.stack.push(1)
			return nil
		}
		n.stack.push(0)
		return nil
	},
	'`': func(n *n3310) error {
		a, b := n.stack.pop(), n.stack.pop()
		if b > a {
			n.stack.push(1)
			return nil
		}
		n.stack.push(0)
		return nil
	},
	'|': func(n *n3310) error {
		a := n.stack.pop()
		if a == 0 {
			n.direction = 2
			return nil
		}
		n.direction = 3
		return nil
	},
	'\\': func(n *n3310) error {
		a, b := n.stack.pop(), n.stack.pop()
		n.stack.push(a)
		n.stack.push(b)
		return nil
	},
	// Stack Functions
	':': func(n *n3310) error {
		n.stack.push(n.stack.peek())
		return nil
	},
	'$': func(n *n3310) error {
		n.stack.pop()
		return nil
	},
	'_': func(n *n3310) error {
		a := n.stack.pop()
		if a == 0 {
			n.direction = 0
			return nil
		}
		n.direction = 1
		return nil
	},

	// Memory Functions
	'g': func(n *n3310) error {
		x, y := int(n.stack.pop()), int(n.stack.pop())
		n.stack.push(n.memory[indexFromPosition(x, y)])
		return nil
	},
	'p': func(n *n3310) error {
		x, y, v := int(n.stack.pop()), int(n.stack.pop()), n.stack.pop()
		n.memory[indexFromPosition(x, y)] = v
		return nil
	},
	'#': func(n *n3310) error {
		n.updatePosition()
		return nil
	},
	'\'': func(n *n3310) error { // New Function: pushes the next character in memory to the stack and then skips it (Replacement for stringmode)
		pos := n.pos
		switch n.direction {
		case 0:
			pos.x++
		case 1:
			pos.x--
		case 2:
			pos.y++
		case 3:
			pos.y--
		}
		n.stack.push(n.memory[indexFromPosition(int(pos.x), int(pos.y))])
		n.updatePosition()
		return nil
	},
	'j': func(n *n3310) error { // New instruction: pops x and y, jumps to position (x,y) in memory
		x, y := n.stack.pop(), n.stack.pop()
		pos := n.addressLabels[byte(indexFromPosition(int(x), int(y)))]
		n.pos.x = pos.x
		n.pos.y = pos.y
		return nil
	},
	// Drawing Functions
	'.': func(n *n3310) error { // Changed from Befunge: Now Pops X and Y, and then draws a single pixel at the position (X,Y)
		x, y := int(n.stack.pop()), int(n.stack.pop())
		n.frameBuffer[indexFromPosition(x, y)] = 1
		return nil
	},
	',': func(n *n3310) error { // Changed from Befunge: Now Pops X1, Y1, X2, Y2 and H, and draws the sprite located at memory(X1,Y1) with H height in the position (X2,Y2)
		fmt.Printf("%c", n.stack.pop())
		return nil
	},
	'C': func(n *n3310) error { // New Instrucion: Clear Screen
		for i := 0; i < 256*128; i++ {
			n.frameBuffer[i] = 0
		}
		return nil
	},
}

func indexFromPosition(x, y int) int {
	const SIZE_X = 256
	const SIZE_Y = 128
	x = x % SIZE_X
	y = y % SIZE_Y
	return (int(y) * SIZE_X) + int(x)
}

func (n *n3310) updatePosition() {
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

func (n *n3310) RunCycle() {
	index := (int(n.pos.y) * 256) + int(n.pos.x)
	inst, ok := instructionMap[n.memory[index]]
	if !ok {
		if n.memory[index] >= '0' && n.memory[index] <= '9' {
			n.stack.push(n.memory[index] - '0')
		} else if n.memory[index] >= 'a' && n.memory[index] <= 'f' {
			n.stack.push(n.memory[index] - 'a')
		}
	} else {
		inst(n)
	}
	n.updatePosition()
}

func (n *n3310) loadInLabels() {
	// CLEARS OUT OLD LABELS
	for k := range n.addressLabels {
		delete(n.addressLabels, k)
	}
	for yPos := 0; yPos < 128; yPos++ {
		for xPos := 1; xPos < 256; xPos++ {
			index := indexFromPosition(xPos, yPos)
			labelIndex := indexFromPosition(xPos-1, yPos)
			if n.memory[index] == ';' {
				c := n.memory[labelIndex]
				n.addressLabels[c] = newPosition(byte(xPos-1), byte(yPos))
			}
		}
	}
}

func (n *n3310) InitializeNotkia() {
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
		n.frameBuffer[i] = 0
	}
	for i := 0; i < 128*64; i++ {
		n.memory[i] = 0
	}
	n.addressLabels = make(map[byte]position)
}

func (n *n3310) ReadCode(romName string) {
	rom, err := os.Open(romName)
	if err != nil {
		panic(err)
	}
	reader := bufio.NewReader(rom)
	buf := make([]byte, 1)
	i := 0
	for {
		_, err := reader.Read(buf)
		if err != nil && !errors.Is(err, io.EOF) {
			panic(err)
		}
		b := buf[0]
		n.memory[i] = b
		i++
		if err != nil {
			// end of file
			break
		}
	}
}
func main() {
	var n n3310
	n.InitializeNotkia()
	n.loadInLabels()
	//n.ReadCode("code")
	n.RunCycle()
	n.RunCycle()
	n.RunCycle()
	n.RunCycle()
	n.RunCycle()

}
