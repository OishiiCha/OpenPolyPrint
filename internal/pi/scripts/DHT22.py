#!/usr/bin/env python3
"""DHT22 temperature/humidity sensor driver using the pigpio daemon.

Reads the DHT22 (AM2302) single-wire protocol through pigpiod, which provides
DMA-accurate microsecond edge timestamps and works reliably inside
Docker containers (where sysfs/mem GPIO access often fails).

The DHT22 protocol is similar to DHT11 but sends 16-bit values for
humidity and temperature (supporting decimals and negative temperatures),
followed by an 8-bit checksum.

Usage:
    import pigpio
    from DHT22 import DHT22

    pi = pigpio.pi()
    sensor = DHT22(pi, 4)
    result = sensor.read()  # {"temperature": 22.5, "humidity": 41.2} or None
    sensor.close()
    pi.stop()
"""

import time

import pigpio


class DHT22(object):
    """Reads relative humidity and temperature from a DHT22/AM2302 sensor."""

    def __init__(self, pi, gpio):
        self.pi = pi
        self.gpio = gpio
        self._edges = []
        # Reason of the last failed read: "no_signal" (nothing on the pin),
        # "incomplete" (partial response), "checksum" (corrupted data) or
        # "range" (values outside DHT22 limits). None after a good read.
        self.status = None
        self.pi.set_pull_up_down(self.gpio, pigpio.PUD_UP)
        self.cb = pi.callback(self.gpio, pigpio.EITHER_EDGE, self._edge)

    def _edge(self, gpio, level, tick):
        self._edges.append((level, tick))
        if len(self._edges) > 200:
            del self._edges[:-200]

    def read(self, timeout=0.5):
        """Trigger one measurement.

        Returns {"temperature": t, "humidity": h} (Celsius, %RH) or None
        when the read failed; in that case self.status says why.
        """
        self.status = None
        # Start pulse: pull the data line low for >=1 ms (DHT22 needs >=1ms,
        # some datasheets say 18ms for AM2302 — use 20ms to be safe).
        self.pi.set_mode(self.gpio, pigpio.OUTPUT)
        self.pi.write(self.gpio, pigpio.LOW)
        time.sleep(0.020)
        self._edges = []
        self.pi.set_mode(self.gpio, pigpio.INPUT)

        deadline = time.time() + timeout
        while time.time() < deadline and len(self._edges) < 86:
            time.sleep(0.005)

        edges = list(self._edges)
        # After release the pull-up alone produces at most one rising edge;
        # nothing else moving means no sensor is driving the line.
        if len(edges) < 3:
            self.status = "no_signal"
            return None
        return self._decode(edges)

    def _decode(self, edges):
        # The sensor answers with an 80us low + 80us high acknowledge,
        # then 40 data bits: each bit is ~50us low followed by a high
        # pulse of ~26us (0) or ~70us (1). The last 40 high pulses are
        # the data bits.
        highs = []
        for i in range(1, len(edges)):
            if edges[i][0] == 0 and edges[i - 1][0] == 1:
                highs.append(pigpio.tickDiff(edges[i - 1][1], edges[i][1]))
        if len(highs) < 41:
            self.status = "incomplete"
            return None

        bits = [1 if w >= 50 else 0 for w in highs[-40:]]

        # DHT22 sends 5 bytes: humidity high, humidity low,
        # temp high, temp low, checksum
        data = bytearray()
        for i in range(0, 40, 8):
            v = 0
            for bit in bits[i:i + 8]:
                v = (v << 1) | bit
            data.append(v)

        if (data[0] + data[1] + data[2] + data[3]) & 0xFF != data[4]:
            self.status = "checksum"
            return None

        # DHT22 humidity: 16-bit big-endian, 1 decimal place
        humidity = ((data[0] << 8) | data[1]) / 10.0

        # DHT22 temperature: 16-bit big-endian, MSB bit 7 = sign
        temp_raw = ((data[2] & 0x7F) << 8) | data[3]
        temperature = temp_raw / 10.0
        if data[2] & 0x80:
            temperature = -temperature

        if humidity > 100 or temperature > 85 or temperature < -40:
            self.status = "range"
            return None
        return {"temperature": temperature, "humidity": humidity}

    def close(self):
        try:
            self.cb.cancel()
        except Exception:
            pass
