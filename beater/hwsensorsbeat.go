package beater

/*
#include <sys/types.h>
#include <sys/sysctl.h>
#include <sys/sensors.h>
*/
import "C"

import (
	"fmt"
	"strings"
	"syscall"
	"time"
	"unsafe"

	"github.com/elastic/beats/libbeat/beat"
	"github.com/elastic/beats/libbeat/cfgfile"
	"github.com/elastic/beats/libbeat/common"
	"github.com/elastic/beats/libbeat/logp"

	"github.com/jasperla/hwsensorsbeat/config"
)

//import "github.com/davecgh/go-spew/spew"

type Hwsensorsbeat struct {
	beatConfig *config.Config
	done       chan struct{}
	period     time.Duration
}

type Sensordev struct {
	num           int32
	xname         [16]byte
	maxnumt       [C.SENSOR_MAX_TYPES]int32
	sensors_count int32
}

type Timeval struct {
	tv_sec  C.time_t
	tv_usec C.suseconds_t
}

type Sensor struct {
	desc        [32]byte
	tv          Timeval
	value       int64
	sensor_type C.enum_sensor_type
	status      C.enum_sensor_status
	numt        int32
	flags       int32
}

type SensorData struct {
	device      string // pchtemp0
	sensor      string // temp0 (type + numt)
	sensor_type string // description uit sensor_types
	unit        string // "degC"
	value       int64  // adjusted to match the unit
	raw_unit    string // uK
	raw_value   int64  // raw value
	status      string // OK, WARNING, etc
	description string // optional description as set by driver
}

// Creates beater
func New() *Hwsensorsbeat {
	return &Hwsensorsbeat{
		done: make(chan struct{}),
	}
}

const selector = "hwsensorsbeat"

/// *** Beater interface methods ***///

func (bt *Hwsensorsbeat) Config(b *beat.Beat) error {

	// Load beater beatConfig
	err := cfgfile.Read(&bt.beatConfig, "")
	if err != nil {
		return fmt.Errorf("Error reading config file: %v", err)
	}

	logp.Debug(selector, "Init hwsensorsbeat")
	logp.Debug(selector, "Period %v", bt.period)

	return nil
}

func (bt *Hwsensorsbeat) Setup(b *beat.Beat) error {

	// Setting default period if not set
	if bt.beatConfig.Hwsensorsbeat.Period == "" {
		bt.beatConfig.Hwsensorsbeat.Period = "1s"
	}

	if bt.beatConfig.Hwsensorsbeat.Devices == nil {
		logp.Debug(selector, "Empty Devices list, collecting all")
	}

	var err error
	bt.period, err = time.ParseDuration(bt.beatConfig.Hwsensorsbeat.Period)
	if err != nil {
		return err
	}

	return nil
}

func (bt *Hwsensorsbeat) Run(b *beat.Beat) error {
	logp.Info("hwsensorsbeat is running! Hit CTRL-C to stop it.")

	mib := [5]int32{C.CTL_HW, C.HW_SENSORS, 0, 0, 0}

	ticker := time.NewTicker(bt.period)
	counter := 1
	for {
		select {
		case <-bt.done:
			return nil
		case <-ticker.C:
		}

		for dev := 0; ; dev += 1 {
			mib[2] = int32(dev)

			sensordev := Sensordev{}
			sensor := Sensor{}

			n := uintptr(0)
			_, _, errno := syscall.Syscall6(syscall.SYS___SYSCTL,
				uintptr(unsafe.Pointer(&mib[0])), 3, 0, uintptr(unsafe.Pointer(&n)), 0, 0)
			// Nothing to see here, moving along.
			if errno == syscall.ENXIO {
				continue
			}

			// Reached the end of available sensors or otherwise a terminating error
			if errno == syscall.ENOENT || errno != 0 || n == 0 {
				break
			}

			_, _, errno = syscall.Syscall6(syscall.SYS___SYSCTL,
				uintptr(unsafe.Pointer(&mib[0])), 3, uintptr(unsafe.Pointer(&sensordev)), uintptr(unsafe.Pointer(&n)), 0, 0)
			if errno != 0 || n == 0 {
				panic("Failed to execute for sysctl")
			}

			// Now that we have the sensordev filled out, we can
			// test to see if hwsensorsbeat was configured to get
			// data for this device. Unless it was requested that
			// all devices are probed.
			if bt.beatConfig.Hwsensorsbeat.Devices != nil {
				// Turn the config into a map so we can more
				// easily check for keys (devices).
				devlist := make(map[string]bool)
				for _, v := range bt.beatConfig.Hwsensorsbeat.Devices {
					devlist[v] = true
				}
				if !devlist[strings.Trim(string(sensordev.xname[:]), "\x00")] {
					continue
				}
			}

			for sensor_type := 0; sensor_type < C.SENSOR_MAX_TYPES; sensor_type += 1 {
				mib[3] = int32(sensor_type)

				for numt := 0; int32(numt) < sensordev.maxnumt[sensor_type]; numt += 1 {
					mib[4] = int32(numt)

					_, _, errno = syscall.Syscall6(syscall.SYS___SYSCTL,
						uintptr(unsafe.Pointer(&mib[0])), 5, 0, uintptr(unsafe.Pointer(&n)), 0, 0)
					if errno != 0 || n == 0 {
						panic("failed to allocate memory for sysctl(5)")
					}

					_, _, errno = syscall.Syscall6(syscall.SYS___SYSCTL,
						uintptr(unsafe.Pointer(&mib[0])), 5, uintptr(unsafe.Pointer(&sensor)), uintptr(unsafe.Pointer(&n)), 0, 0)

					if errno != 0 || n == 0 {
						panic("failed to execute sysctl(5)")
					}

					// Now try to fetch the actual sensor data
					_, _, errno = syscall.Syscall6(syscall.SYS___SYSCTL,
						uintptr(unsafe.Pointer(&mib[0])), 5, 0, uintptr(unsafe.Pointer(&n)), 0, 0)
					if errno != 0 || n == 0 {
						panic("failed to allocate memory for sysctl(5)")
					}

					_, _, errno = syscall.Syscall6(syscall.SYS___SYSCTL,
						uintptr(unsafe.Pointer(&mib[0])), 5, uintptr(unsafe.Pointer(&sensor)), uintptr(unsafe.Pointer(&n)), 0, 0)
					if errno != 0 || n == 0 {
						panic("failed to execute sysctl(5)")
					}

					if sensor.flags&C.SENSOR_FINVALID > 0 {
						break
					}

					s := resolve_sensor(&sensordev, &sensor)

					event := common.MapStr{
						"@timestamp":  common.Time(time.Now()),
						"type":        "hw.sensors",
						"counter":     counter,
						"device":      s.device,
						"sensor":      s.sensor,
						"sensor_type": s.sensor_type,
						"unit":        s.unit,
						"value":       s.value,
						"raw_unit":    s.raw_unit,
						"raw_value":   s.raw_value,
						"status":      s.status,
						"description": s.description,
					}
					b.Events.PublishEvent(event)
					logp.Info("Event sent")
					counter++
				}
			}
		}

	}

	return nil
}

func (bt *Hwsensorsbeat) Cleanup(b *beat.Beat) error {
	return nil
}

func (bt *Hwsensorsbeat) Stop() {
	close(bt.done)
}

func resolve_sensor_status(sensor *Sensor) string {
	status := ""

	switch sensor.status {
	case C.SENSOR_S_OK:
		status = "OK"
	case C.SENSOR_S_WARN:
		status = "WARNING"
	case C.SENSOR_S_CRIT:
		status = "CRITICAL"
	case C.SENSOR_S_UNKNOWN:
		status = "UNKNOWN"
	case C.SENSOR_S_UNSPEC:
		status = ""
	}

	return status
}

func resolve_sensor_type(sensor *Sensor) string {
	sensor_types := [C.SENSOR_MAX_TYPES + 1]string{
		"temp",
		"fan",
		"volt",
		"acvolt",
		"resistance",
		"power",
		"current",
		"watthour",
		"amphour",
		"indicator",
		"raw",
		"percent",
		"illuminance",
		"drive",
		"timedelta",
		"humidity",
		"frequency",
		"angle",
		"distance",
		"pressure",
		"acceleration",
		"undefined",
	}

	return sensor_types[sensor.sensor_type]
}

// Take the raw sensor/sensordev and return the struct with only fields we care about
func resolve_sensor(sensordev *Sensordev, sensor *Sensor) SensorData {
	s := SensorData{}

	// Convert from byte slice to string and zap all trailing NUL characters
	s.device = strings.Trim(string(sensordev.xname[:]), "\x00")
	s.description = strings.Trim(string(sensor.desc[:]), "\x00")
	stype := resolve_sensor_type(sensor)
	s.sensor = fmt.Sprintf("%s%d", stype, sensor.numt)
	s.sensor_type = stype
	s.raw_value = sensor.value
	s.status = resolve_sensor_status(sensor)

	// Conversions from sensorsd.c:
	// Copyright (c) 2003 Henning Brauer <henning@openbsd.org>
	// Copyright (c) 2005 Matthew Gream <matthew.gream@pobox.com>
	// Copyright (c) 2006 Constantine A. Murenin <cnst+openbsd@bugmail.mojo.ru>
	switch sensor.sensor_type {
	case C.SENSOR_TEMP:
		s.value = (sensor.value - 273150000) / 1000000.0
		s.unit = "degC"
		s.raw_unit = "uK"
		break
	case C.SENSOR_FANRPM:
		s.value = sensor.value
		s.unit = "RPM"
		s.raw_unit = s.unit
		break
	case C.SENSOR_VOLTS_DC:
		s.value = sensor.value / 1000000.0
		s.unit = "V DC"
		s.raw_unit = "uV DC"
		break
	case C.SENSOR_VOLTS_AC:
		s.value = sensor.value / 1000000.0
		s.unit = "V AC"
		s.raw_unit = "uV AC"
		break
	case C.SENSOR_OHMS:
		s.value = sensor.value / 1000000.0
		s.unit = "Ohms"
		s.raw_unit = "uOhms"
		break
	case C.SENSOR_WATTS:
		s.value = sensor.value / 1000000.0
		s.unit = "W"
		s.raw_unit = "uW"
		break
	case C.SENSOR_AMPS:
		s.value = sensor.value / 1000000.0
		s.unit = "A"
		s.raw_unit = "uA"
		break
	case C.SENSOR_WATTHOUR:
		s.value = sensor.value / 1000000.0
		s.unit = "Wh"
		s.raw_unit = "uWh"
		break
	case C.SENSOR_AMPHOUR:
		s.value = sensor.value / 1000000.0
		s.unit = "Ah"
		s.raw_unit = "uAh"
		break
	case C.SENSOR_INDICATOR:
		s.value = sensor.value
		s.unit = "boolean"
		s.raw_unit = s.unit
		break
	case C.SENSOR_INTEGER:
		s.value = sensor.value
		s.unit = ""
		s.raw_unit = ""
		break
	case C.SENSOR_PERCENT:
		s.value = sensor.value / 1000.0
		s.unit = "%"
		s.raw_unit = "m%"
		break
	case C.SENSOR_LUX:
		s.value = sensor.value / 1000000.0
		s.unit = "lx"
		s.raw_unit = "ulx"
		break
	case C.SENSOR_DRIVE:
		s.value = sensor.value
		s.unit = ""
		s.raw_unit = ""
		break
	case C.SENSOR_TIMEDELTA:
		s.value = sensor.value / 1000000000.0
		s.unit = "secs"
		s.raw_unit = "nSec"
		break
	case C.SENSOR_HUMIDITY:
		s.value = sensor.value / 1000.0
		s.unit = "%RH"
		s.raw_unit = "m%RH"
		break
	case C.SENSOR_FREQ:
		s.value = sensor.value / 1000000.0
		s.unit = "Hz"
		s.raw_unit = "uHz"
		break
	case C.SENSOR_ANGLE:
		s.value = sensor.value
		s.unit = "uDegrees"
		s.raw_unit = s.unit
		break
	case C.SENSOR_DISTANCE:
		s.value = sensor.value / 1000.0
		s.unit = "mm"
		s.raw_unit = "uMeter"
		break
	case C.SENSOR_PRESSURE:
		s.value = sensor.value / 1000.0
		s.unit = "Pa"
		s.raw_unit = "mPa"
		break
	case C.SENSOR_ACCEL:
		s.value = sensor.value / 1000000.0
		s.unit = "m/s^2"
		s.raw_unit = "u m/s^2"
		break
	default:
		s.value = sensor.value
		s.raw_unit = "???"
		s.unit = "???"
	}

	return s
}
