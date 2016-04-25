# hwsensorsbeat

Welcome to hw.sensors beat. This beat reads the `hw.sensors` information exported
by sysctl(3) on OpenBSD.

Sample output (via sysctl(8)):

```
hw.sensors.cpu0.temp0=40.00 degC
hw.sensors.acpitz0.temp0=40.00 degC (zone temperature)
hw.sensors.acpibtn0.indicator0=On (lid open)
hw.sensors.acpibat0.volt0=11.10 VDC (voltage)
hw.sensors.acpibat0.volt1=12.35 VDC (current voltage)
hw.sensors.acpibat0.power0=0.00 W (rate)
hw.sensors.acpibat0.watthour0=19.83 Wh (last full capacity)
hw.sensors.acpibat0.watthour1=0.99 Wh (warning capacity)
hw.sensors.acpibat0.watthour2=0.20 Wh (low capacity)
hw.sensors.acpibat0.watthour3=19.79 Wh (remaining capacity), OK
hw.sensors.acpibat0.watthour4=23.20 Wh (design capacity)
hw.sensors.acpibat0.raw0=0 (battery idle), OK
hw.sensors.acpibat1.volt0=11.10 VDC (voltage)
hw.sensors.acpibat1.volt1=12.35 VDC (current voltage)
hw.sensors.acpibat1.power0=0.00 W (rate)
hw.sensors.acpibat1.watthour0=19.40 Wh (last full capacity)
hw.sensors.acpibat1.watthour1=0.97 Wh (warning capacity)
hw.sensors.acpibat1.watthour2=0.20 Wh (low capacity)
hw.sensors.acpibat1.watthour3=19.28 Wh (remaining capacity), OK
hw.sensors.acpibat1.watthour4=23.20 Wh (design capacity)
hw.sensors.acpibat1.raw0=0 (battery idle), OK
hw.sensors.acpiac0.indicator0=On (power supply)
hw.sensors.acpithinkpad0.fan0=0 RPM
hw.sensors.pchtemp0.temp0=44.00 degC
```

### Build

To build the binary for hwsensorsbeat run the command below. This will generate a binary
in the same directory with the name hwsensorsbeat.

```
make
```


### Run

To run hwsensorsbeat with debugging output enabled, run:

```
./hwsensorsbeat -c hwsensorsbeat.yml -e -d "*"
```

There is no need to load the template/mapping, all types used are
primitive Elastic types and are deduced correctly (string/long)

### Test

To test Hwsensorsbeat, run the following command:

```
make testsuite
```

alternatively:
```
make unit-tests
make system-tests
make integration-tests
make coverage-report
```

The test coverage is reported in the folder `./build/coverage/`


### Package

To cross-compile and package Hwsensorsbeat for all supported platforms, run the following commands:

```
cd dev-tools/packer
make deps
make images
make
```

or on OpenBSD simply run the following command to install the latest
version:

```
pkg_add hwsensorsbeat
```

### Update

Each beat has a template for the mapping in elasticsearch and a documentation for the fields
which is automatically generated based on `etc/fields.yml`.
To generate etc/hwsensorsbeat.template.json and etc/hwsensorsbeat.asciidoc

```
make update
```


### Cleanup

To clean  Hwsensorsbeat source code, run the following commands:

```
make fmt
make simplify
```

To clean up the build directory and generated artifacts, run:

```
make clean
```


### Clone

To clone Hwsensorsbeat from the git repository, run the following commands:

```
mkdir -p ${GOPATH}/github.com/jasperla
cd ${GOPATH}/github.com/jasperla
git clone https://github.com/jasperla/hwsensorsbeat
```


For further development, check out the [beat developer guide](https://www.elastic.co/guide/en/beats/libbeat/current/new-beat.html).
