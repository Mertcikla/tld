package iot

import (
	"github.com/mertcikla/tld/internal/watch/enrich"
	"github.com/mertcikla/tld/internal/watch/enrich/enrichers/pattern"
)

func All() []enrich.Enricher { return pattern.FromSpecs(Specs()) }

func Specs() []pattern.Spec {
	return []pattern.Spec{
		spec("ts.mqtt", "TypeScript MQTT", "typescript", "mqtt", "mqtt.connect", "iot.mqtt_topic", "publishes_to_device"),
		spec("python.mqtt", "Python MQTT", "python", "paho-mqtt", "paho.mqtt", "iot.mqtt_topic", "publishes_to_device"),
		spec("go.mqtt", "Go MQTT", "go", "github.com/eclipse/paho.mqtt.golang", "mqtt.NewClient", "iot.mqtt_topic", "publishes_to_device"),
		spec("cpp.mqtt", "C++ MQTT", "cpp", "paho.mqtt.cpp", "mqtt::client", "iot.mqtt_topic", "publishes_to_device"),
		spec("ts.coap", "TypeScript CoAP", "typescript", "coap", "coap.request", "iot.broker", "publishes_to_device"),
		spec("python.coap", "Python CoAP", "python", "aiocoap", "aiocoap", "iot.broker", "publishes_to_device"),
		spec("go.coap", "Go CoAP", "go", "github.com/plgd-dev/go-coap", "coap", "iot.broker", "publishes_to_device"),
		spec("cpp.coap", "C++ CoAP", "cpp", "libcoap", "coap_", "iot.broker", "publishes_to_device"),
		spec("python.i2c", "Python I2C", "python", "smbus", "smbus", "hardware.bus_address", "communicates_via_i2c"),
		spec("rust.i2c", "Rust I2C", "rust", "embedded-hal", "i2c", "hardware.bus_address", "communicates_via_i2c"),
		spec("cpp.i2c", "C++ I2C", "cpp", "i2c", "ioctl", "hardware.bus_address", "communicates_via_i2c"),
		spec("python.spi", "Python SPI", "python", "spidev", "spidev", "hardware.bus_address", "communicates_via_i2c"),
		spec("rust.spi", "Rust SPI", "rust", "embedded-hal", "spi", "hardware.bus_address", "communicates_via_i2c"),
		spec("cpp.spi", "C++ SPI", "cpp", "spi", "SPI_IOC", "hardware.bus_address", "communicates_via_i2c"),
		spec("ts.uart", "TypeScript UART", "typescript", "serialport", "SerialPort", "hardware.pin", "communicates_via_i2c"),
		spec("python.uart", "Python UART", "python", "pyserial", "serial.Serial", "hardware.pin", "communicates_via_i2c"),
		spec("cpp.uart", "C++ UART", "cpp", "uart", "termios", "hardware.pin", "communicates_via_i2c"),
		spec("python.can_bus", "Python CAN Bus", "python", "python-can", "can.Bus", "hardware.bus_address", "communicates_via_i2c"),
		spec("rust.can_bus", "Rust CAN Bus", "rust", "socketcan", "CANSocket", "hardware.bus_address", "communicates_via_i2c"),
		spec("cpp.can_bus", "C++ CAN Bus", "cpp", "socketcan", "PF_CAN", "hardware.bus_address", "communicates_via_i2c"),
	}
}

func spec(id, name, language, dependency, token, factType, relationship string) pattern.Spec {
	return pattern.Spec{
		ID:           id,
		Name:         name,
		Category:     "iot",
		Languages:    []string{language},
		Mode:         enrich.ActivationImportOrDependency,
		Triggers:     []enrich.ActivationSignal{{Kind: enrich.SignalDependency, Value: dependency}, {Kind: enrich.SignalImport, Value: dependency}},
		FactType:     factType,
		Relationship: relationship,
		SourceTokens: []string{token},
		Tags:         []string{"iot:" + id},
		Attributes:   map[string]string{"dependency": dependency, "language": language},
	}
}
