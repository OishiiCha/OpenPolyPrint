#!/usr/bin/env python3
"""DHT11 sensor reader for OpenPolyPrint filament box monitoring.
Reads temperature and humidity from a DHT11 sensor on a specified GPIO pin.
Outputs JSON to stdout: {"temp": 25.0, "humidity": 45.0, "error": null}
On error: {"temp": null, "humidity": null, "error": "message"}

Usage: python3 dht11_reader.py <bcm_pin>
"""

import json
import sys
import time
from pathlib import Path

# Make the vendored DHT11.py driver importable. It lives next to this script,
# both in the repo and in the temp directory the Go server extracts us to.
sys.path.insert(0, str(Path(__file__).resolve().parent))


def _err(msg):
    return {"temp": None, "humidity": None, "error": msg}


def read_with_pigpio(pin):
    import pigpio
    from DHT11 import DHT11

    pi = pigpio.pi()
    if not pi.connected:
        return _err("pigpiod not connected")
    try:
        sensor = DHT11(pi, pin)
        try:
            # DHT11 needs >=1s between reads; a first read after idle
            # frequently fails the checksum, so retry.
            statuses = []
            for _ in range(3):
                result = sensor.read()
                if result is not None:
                    return {
                        "temp": round(result["temperature"], 1),
                        "humidity": round(result["humidity"], 1),
                        "error": None,
                    }
                statuses.append(sensor.status or "unknown")
                time.sleep(1.2)
            if all(s == "no_signal" for s in statuses):
                return _err("no signal on GPIO%d - sensor not connected or wrong pin" % pin)
            return _err("no valid reading from GPIO%d (%s) - check wiring and pull-up"
                        % (pin, "/".join(sorted(set(statuses)))))
        finally:
            sensor.close()
    finally:
        pi.stop()


def read_with_adafruit(pin):
    import Adafruit_DHT
    import Adafruit_DHT.platform_detect as pd
    # Force Raspberry Pi platform detection for Docker environments
    # where /proc/cpuinfo doesn't expose Pi hardware info. Select the Pi 2
    # driver (its C code reads the real peripheral base from the device
    # tree, so it works on Pi 2/3/4).
    pd.platform_detect = lambda: pd.RASPBERRY_PI
    pd.pi_revision = lambda: 2
    humidity, temperature = Adafruit_DHT.read_retry(Adafruit_DHT.DHT11, pin, retries=3, delay_seconds=1)
    if humidity is not None and temperature is not None:
        return {"temp": round(temperature, 1), "humidity": round(humidity, 1), "error": None}
    return _err("Adafruit_DHT read failed")


def main():
    if len(sys.argv) < 2:
        print(json.dumps(_err("missing pin argument")))
        sys.exit(1)

    pin = int(sys.argv[1])

    # Try libraries in order of preference
    errors = []
    for reader in [read_with_pigpio, read_with_adafruit]:
        try:
            result = reader(pin)
            if result["error"] is None:
                print(json.dumps(result))
                return
            errors.append(result["error"])
        except ImportError as e:
            errors.append(f"{reader.__name__}: library not installed ({e})")
        except Exception as e:
            errors.append(f"{reader.__name__}: {e}")

    print(json.dumps(_err("; ".join(errors))))


if __name__ == "__main__":
    main()
