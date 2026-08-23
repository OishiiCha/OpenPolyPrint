#!/usr/bin/env python3
"""DHT11 temperature/humidity sensor driver using the pigpio daemon.

Reads the DHT11 single-wire protocol through pigpiod, which provides
DMA-accurate microsecond edge timestamps and works reliably inside
Docker containers (where sysfs/mem GPIO access often fails).

This is a vendored replacement for the pigpio example dht11.py, whose
master version only implements the Python 2 style next() method.

Usage:
    import pigpio
    from DHT11 import DHT11

    pi = pigpio.pi()
    sensor = DHT11(pi, 4)
    result = sensor.read()  # {"temperature": 22.0, "humidity": 41.0} or None
    sensor.close()
    pi.stop()
"""

import time

import pigpio


class DHT11(object):
    """Reads relative humidity and temperature from a DHT11 sensor."""

    def __init__(self, pi, gpio):
        self.pi = pi
        self.gpio = gpio
        self._edges = []
        # Reason of the last failed read: "no_signal" (nothing on the pin),
        # "incomplete" (partial response), "checksum" (corrupted data) or
        # "range" (values outside DHT11 limits). None after a good read.
        self.status = None
        self.pi.set_pull_up_down(self.gpio, pigpio.PUD_UP)
        self.cb = pi.callback(self.gpio, pigpio.EITHER_EDGE, self._edge)

    def _edge(self, gpio, level, tick):
        self._edges.append((level, tick))
        if len(self._edges) > 200:
            del self._edges[:-200]

    def read(self, timeout=0.25):
        """Trigger one measurement.

        Returns {"temperature": t, "humidity": h} (Celsius, %RH) or None
        when the read failed; in that case self.status says why.
        """
        self.status = None
        # Start pulse: pull the data line low for >=18 ms, then release.
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
        data = bytearray()
        for i in range(0, 40, 8):
            v = 0
            for bit in bits[i:i + 8]:
                v = (v << 1) | bit
            data.append(v)

        if (data[0] + data[1] + data[2] + data[3]) & 0xFF != data[4]:
            self.status = "checksum"
            return None

        humidity = data[0] + (data[1] & 0x0F) / 10.0
        temperature = data[2] + (data[3] & 0x0F) / 10.0
        if humidity > 100 or temperature > 60:
            self.status = "range"
            return None
        return {"temperature": temperature, "humidity": humidity}

    def close(self):
        try:
            self.cb.cancel()
        except Exception:
            pass
