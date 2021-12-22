package main

import (
	"context"
	"fmt"
	"math"
	"time"

	"github.com/jackc/pgx/v4/pgxpool"
	"periph.io/x/conn/v3/i2c"
	"periph.io/x/conn/v3/i2c/i2creg"
	"periph.io/x/conn/v3/physic"
	"periph.io/x/devices/v3/mcp9808"
	"periph.io/x/host/v3"
)

func check(err error) {
	if err != nil {
		panic(err)
	}
}

const SensorDevAddr = 0x18

const BLightDevAddr = 0x62
const BLightCtrlAddr = 0x08

const DispDevAddr = 0x3e
const DispCtrlAddr = 0x00
const DispTextAddr = 0x40

var BlightPwm = []byte {0x02, 0x03, 0x04, 0x05}

func initDisplay(display *i2c.Dev) {
	_, _ = display.Write([]byte{DispCtrlAddr, 0x28})

	// Turn display on
	_, _ = display.Write([]byte{DispCtrlAddr, 0x0d})

	// Clear the display
	_, _ = display.Write([]byte{DispCtrlAddr, 0x01})

	_, _ = display.Write([]byte{DispCtrlAddr, 0x06})

	_, _ = display.Write([]byte{DispCtrlAddr, 0x02})

	// Set cursor to first char line 1
	_, _ = display.Write([]byte{DispCtrlAddr, 0x80})

	// Write prompt segment
	promptText := []byte("Temp: ")
	for i := 0; i < len(promptText); i++ {
		_, _ = display.Write([]byte{DispTextAddr, promptText[i]})
	}
}

func initBacklight(backlight *i2c.Dev) {
	// Disable Sleep Mode
	_, _ = backlight.Write([]byte{0x00, 0x00})

	// Enable PWM control
	_, _ = backlight.Write([]byte{BLightCtrlAddr, 0xaa})

	// Set backlight brightness
	_, _ = backlight.Write([]byte{BlightPwm[0], 0xbb})
	_, _ = backlight.Write([]byte{BlightPwm[1], 0xbb})
	_, _ = backlight.Write([]byte{BlightPwm[2], 0xbb})
	_, _ = backlight.Write([]byte{BlightPwm[3], 0xbb})
}

func setBacklightColors(temp float64, backlight *i2c.Dev) {
	switch {
	case temp >= 25:
		_, _ = backlight.Write([]byte{BLightCtrlAddr, 0x10}) // Red
		break
	case temp >= 22.5:
		_, _ = backlight.Write([]byte{BLightCtrlAddr, 0x04}) // Green
		break
	default:
		_, _ = backlight.Write([]byte{BLightCtrlAddr, 0x01}) // Blue
	}
}

func writeTempToDisplay(temp physic.Temperature, display, backlight *i2c.Dev) {
	writeableTemp := []byte(fmt.Sprintf("%f", temp.Celsius()))

	// Move cursor to seventh char line 1
	_, _ = display.Write([]byte{DispCtrlAddr, 0x86})

	setBacklightColors(temp.Celsius(), backlight)
	for i := 0; i < len(writeableTemp[:4]); i++ {
		_, _ = display.Write([]byte{DispTextAddr, writeableTemp[i]})
	}

	// Degree sign
	_, _ = display.Write([]byte{DispTextAddr, 0xdf})
}

func main() {
	_, _ = host.Init()

	// Attempt to open i2c bus
	bus, err := i2creg.Open("/dev/i2c-2")
	check(err)

	display := &i2c.Dev{Bus: bus, Addr: DispDevAddr}
	backlight := &i2c.Dev{Bus: bus, Addr: BLightDevAddr}

	initDisplay(display)
	initBacklight(backlight)

	// Get temp sensor
	sensor, err := mcp9808.New(bus, &mcp9808.Opts{Addr: SensorDevAddr, Res: mcp9808.Medium})

	temp, _ := sensor.SenseTemp()
	writeTempToDisplay(temp, display, backlight)

	ctx := context.Background()
	dbpool, err := pgxpool.Connect(ctx, "postgres://postgres:password@192.168.0.26:5432/postgres")
	check(err)
	defer dbpool.Close()

	insertQuery := "insert into temps (time, kelvin, celsius, fahrenheit) values ($1,$2,$3,$4);"
	sleepTime := 250 * time.Millisecond
	counter := 0

	for ;; {
		time.Sleep(sleepTime)

		temp, _ = sensor.SenseTemp()
		writeTempToDisplay(temp, display, backlight)

		if counter > 4 {
			// Update database with current reading
			_, err = dbpool.Exec(ctx, insertQuery, time.Now(), temp.Celsius() + 274.15, temp.Celsius(), math.Round(temp.Fahrenheit()*100) / 100)
			check(err)

			counter = 0
		} else {
			counter++
		}
	}
}

