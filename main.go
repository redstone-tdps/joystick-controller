package main

import (
	"fmt"
	"strings"

	"joystick-controller/repl"

	evdev "github.com/gvalkov/golang-evdev"
)

const TEST_RP2040_PORT = "/dev/serial/by-id/usb-MicroPython_Board_in_FS_mode_505431655893979c-if00"
const MOTOR_RP2040_PORT = "/dev/serial/by-id/usb-MicroPython_Board_in_FS_mode_e6632c25a31d362d-if00"

func main() {
	motorREPL, err := repl.New(TEST_RP2040_PORT, repl.DefaultBaud, repl.DefaultTimeout, false)
	if err != nil {
		fmt.Printf("Failed to connect to motor RP2040 on %s: %v\n", TEST_RP2040_PORT, err)
		return
	}
	defer motorREPL.Close()

	if err := motorREPL.EnterRawREPL(); err != nil {
		fmt.Printf("Failed to enter raw REPL: %v\n", err)
		return
	}
	defer motorREPL.ExitRawREPL()

	devices, _ := evdev.ListInputDevices()
	var controller *evdev.InputDevice

	for _, dev := range devices {
		if dev.Name == "Xbox Wireless Controller" || dev.Name == "Zikway HID gamepad" {
			controller = dev
			break
		}
		fmt.Println(dev.Name)
	}

	if controller == nil {
		fmt.Println("Controller not found. Please connect your Xbox controller and try again.")
		return
	}

	fmt.Printf("Listening: %s\n", controller.Name)

	const Center = 128
	const Deadzone = 20
	var curX, curY int32
	var lastM1, lastM2, lastM3, lastM4 int

	fmt.Println("Listening for directions...")

	for {
		event, err := controller.ReadOne()
		if err != nil {
			break
		}

		if event.Type == evdev.EV_ABS {
			if event.Code == 2 {
				curX = event.Value
			} else if event.Code == 5 {
				curY = event.Value
			} else {
				continue
			}

			var horiz, vert string

			// Horizontal Logic (Code 2)
			if curX < (Center - Deadzone) {
				horiz = "left"
			} else if curX > (Center + Deadzone) {
				horiz = "right"
			}

			// Vertical Logic (Code 5)
			if curY < (Center - Deadzone) {
				vert = "up"
			} else if curY > (Center + Deadzone) {
				vert = "down"
			}

			// Combine strings
			direction := strings.TrimSpace(horiz + " " + vert)

			xPct := axisToPercent(curX, Center, Deadzone)
			yPct := axisToPercent(curY, Center, Deadzone)

			// Up on the stick reduces Y value, so invert for forward throttle.
			throttle := -yPct
			steering := xPct

			left := clamp(throttle+steering, -100, 100)
			right := clamp(throttle-steering, -100, 100)

			m1, m2, m3, m4 := left, right, left, right
			if m1 != lastM1 || m2 != lastM2 || m3 != lastM3 || m4 != lastM4 {
				cmd := fmt.Sprintf("set_speeds(%d,%d,%d,%d)", m1, m2, m3, m4)
				_, stderr, execErr := motorREPL.ExecRaw(cmd)
				if execErr != nil {
					fmt.Printf("\nMotor command failed: %v\n", execErr)
				} else if stderr != "" {
					fmt.Printf("\nMotor REPL stderr: %s\n", stderr)
				} else {
					lastM1, lastM2, lastM3, lastM4 = m1, m2, m3, m4
				}
			}

			if direction != "" {
				fmt.Printf("\rCurrent Direction: %-15s (X:%d Y:%d) Speeds:[%d %d %d %d]   ", direction, curX, curY, m1, m2, m3, m4)
			} else {
				fmt.Printf("\rCurrent Direction: centered        (X:%d Y:%d) Speeds:[%d %d %d %d]   ", curX, curY, m1, m2, m3, m4)
			}
		}
	}
}

func axisToPercent(value int32, center int, deadzone int) int {
	delta := int(value) - center
	if abs(delta) <= deadzone {
		return 0
	}

	max := center - deadzone
	if delta > 0 {
		return clamp((delta-deadzone)*100/max, -100, 100)
	}
	return clamp((delta+deadzone)*100/max, -100, 100)
}

func clamp(value int, min int, max int) int {
	if value < min {
		return min
	}
	if value > max {
		return max
	}
	return value
}

func abs(value int) int {
	if value < 0 {
		return -value
	}
	return value
}
