package main

import (
	"fmt"
	"strings"
	"time"

	"joystick-controller/repl"

	evdev "github.com/gvalkov/golang-evdev"
)

const (
	TEST_RP2040_PORT  = "/dev/serial/by-id/usb-MicroPython_Board_in_FS_mode_505431655893979c-if00"
	MOTOR_RP2040_PORT = "/dev/serial/by-id/usb-MicroPython_Board_in_FS_mode_e6632c25a31d362d-if00"

	ControllerRetryDelay = 2 * time.Second
	SerialRetryDelay     = 2 * time.Second
)

func main() {
	motorREPL := waitForMotorREPL(MOTOR_RP2040_PORT)
	controller := waitForController()

	const Center = 128
	const Deadzone = 20
	var curX, curY int32
	var lastM1, lastM2, lastM3, lastM4 int
	haveLastSpeeds := false

	fmt.Println("Listening for directions...")

	for {
		event, err := controller.ReadOne()
		if err != nil {
			fmt.Printf("\nController read error: %v\n", err)
			controller = waitForController()
			continue
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

			left := clamp(throttle-steering, -100, 100) * 5
			right := clamp(throttle+steering, -100, 100) * 5

			m1, m2, m3, m4 := left, right, left, right
			if !haveLastSpeeds || m1 != lastM1 || m2 != lastM2 || m3 != lastM3 || m4 != lastM4 {
				cmd := fmt.Sprintf("set_speeds(%d,%d,%d,%d)", m1, m2, m3, m4)
				_, stderr, execErr := motorREPL.ExecRaw(cmd)
				if execErr != nil {
					fmt.Printf("\nMotor command failed: %v\n", execErr)
					closeMotorREPL(motorREPL)
					motorREPL = waitForMotorREPL(MOTOR_RP2040_PORT)
					haveLastSpeeds = false
				} else if stderr != "" {
					fmt.Printf("\nMotor REPL stderr: %s\n", stderr)
				} else {
					lastM1, lastM2, lastM3, lastM4 = m1, m2, m3, m4
					haveLastSpeeds = true
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

func waitForMotorREPL(port string) *repl.MicroPythonREPL {
	attempt := 1
	for {
		motorREPL, err := repl.New(port, repl.DefaultBaud, repl.DefaultTimeout, false)
		if err != nil {
			fmt.Printf("Motor connect failed (attempt %d): %v\n", attempt, err)
			attempt++
			time.Sleep(SerialRetryDelay)
			continue
		}

		if err := motorREPL.EnterRawREPL(); err != nil {
			fmt.Printf("Enter raw REPL failed (attempt %d): %v\n", attempt, err)
			_ = motorREPL.Close()
			attempt++
			time.Sleep(SerialRetryDelay)
			continue
		}

		fmt.Printf("Connected to motor RP2040 on %s\n", port)
		return motorREPL
	}
}

func closeMotorREPL(motorREPL *repl.MicroPythonREPL) {
	if motorREPL == nil {
		return
	}
	_ = motorREPL.ExitRawREPL()
	_ = motorREPL.Close()
}

func waitForController() *evdev.InputDevice {
	attempt := 1
	for {
		controller, names, err := findController()
		if err != nil {
			fmt.Printf("Controller scan failed (attempt %d): %v\n", attempt, err)
		} else if controller != nil {
			fmt.Printf("Listening: %s\n", controller.Name)
			return controller
		} else {
			fmt.Printf("Controller not found (attempt %d)", attempt)
			if len(names) > 0 {
				fmt.Printf(". Detected devices: %s", strings.Join(names, ", "))
			}
			fmt.Println()
		}

		attempt++
		time.Sleep(ControllerRetryDelay)
	}
}

func findController() (*evdev.InputDevice, []string, error) {
	devices, err := evdev.ListInputDevices()
	if err != nil {
		return nil, nil, err
	}

	names := make([]string, 0, len(devices))
	for _, dev := range devices {
		names = append(names, dev.Name)
		if dev.Name == "Xbox Wireless Controller" || dev.Name == "Zikway HID gamepad" {
			return dev, names, nil
		}
	}

	return nil, names, nil
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
