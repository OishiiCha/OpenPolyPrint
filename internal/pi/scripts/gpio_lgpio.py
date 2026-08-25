#!/usr/bin/env python3
"""GPIO control daemon via lgpio.

This process stays alive and holds GPIO handles open so that output values
persist (needed for relay control). It reads JSON commands from stdin (one
per line) and writes JSON responses to stdout.

Commands:
  {"cmd": "info"}                                    — check if lgpio works
  {"cmd": "mode", "pin": 17, "mode": "w"}            — set pin mode (w=out, r=in)
  {"cmd": "write", "pin": 17, "value": 1}            — write pin value
  {"cmd": "read", "pin": 17}                          — read pin value
  {"cmd": "free", "pin": 17}                          — free a pin
  {"cmd": "quit"}                                     — exit daemon

Responses:
  {"ok": true}                  — success
  {"ok": true, "value": 1}      — success with value (read)
  {"ok": false, "error": "..."} — failure
"""
import sys
import json

def main():
    try:
        import lgpio
    except ImportError as e:
        # Can't even import lgpio — send error and exit
        print(json.dumps({"ok": False, "error": f"lgpio not installed: {e}"}), flush=True)
        sys.exit(1)

    # Try to open gpiochip0
    try:
        h = lgpio.gpiochip_open(0)
        if h < 0:
            print(json.dumps({"ok": False, "error": f"gpiochip_open returned {h}"}), flush=True)
            sys.exit(1)
    except Exception as e:
        print(json.dumps({"ok": False, "error": f"gpiochip_open: {e}"}), flush=True)
        sys.exit(1)

    # Track which pins are claimed and in what mode
    claimed = {}  # pin -> "input" | "output"

    # Signal that we're ready
    print(json.dumps({"ok": True, "msg": "lgpio daemon ready"}), flush=True)

    for line in sys.stdin:
        line = line.strip()
        if not line:
            continue

        try:
            req = json.loads(line)
        except json.JSONDecodeError as e:
            print(json.dumps({"ok": False, "error": f"invalid JSON: {e}"}), flush=True)
            continue

        cmd = req.get("cmd", "")

        try:
            if cmd == "quit":
                # Free all claimed pins
                for pin in list(claimed.keys()):
                    try:
                        lgpio.gpio_free(h, pin)
                    except Exception:
                        pass
                lgpio.gpiochip_close(h)
                print(json.dumps({"ok": True}), flush=True)
                sys.exit(0)

            elif cmd == "info":
                print(json.dumps({"ok": True}), flush=True)

            elif cmd == "mode":
                pin = int(req["pin"])
                mode = req["mode"]  # "w" = output, "r" = input

                # Free pin if already claimed
                if pin in claimed:
                    try:
                        lgpio.gpio_free(h, pin)
                    except Exception:
                        pass
                    del claimed[pin]

                if mode == "w":
                    lgpio.gpio_claim_output(h, pin, 0)
                    claimed[pin] = "output"
                else:
                    lgpio.gpio_claim_input(h, pin)
                    claimed[pin] = "input"

                print(json.dumps({"ok": True}), flush=True)

            elif cmd == "write":
                pin = int(req["pin"])
                value = int(req["value"])

                # Claim as output if not already claimed as output
                if pin not in claimed or claimed[pin] != "output":
                    if pin in claimed:
                        try:
                            lgpio.gpio_free(h, pin)
                        except Exception:
                            pass
                    lgpio.gpio_claim_output(h, pin, value)
                    claimed[pin] = "output"
                else:
                    lgpio.gpio_write(h, pin, value)

                print(json.dumps({"ok": True}), flush=True)

            elif cmd == "read":
                pin = int(req["pin"])

                # Claim as input if not already claimed as input
                if pin not in claimed or claimed[pin] != "input":
                    if pin in claimed:
                        try:
                            lgpio.gpio_free(h, pin)
                        except Exception:
                            pass
                    lgpio.gpio_claim_input(h, pin)
                    claimed[pin] = "input"

                val = lgpio.gpio_read(h, pin)
                print(json.dumps({"ok": True, "value": val}), flush=True)

            elif cmd == "free":
                pin = int(req["pin"])
                if pin in claimed:
                    lgpio.gpio_free(h, pin)
                    del claimed[pin]
                print(json.dumps({"ok": True}), flush=True)

            else:
                print(json.dumps({"ok": False, "error": f"unknown command: {cmd}"}), flush=True)

        except Exception as e:
            print(json.dumps({"ok": False, "error": str(e)}), flush=True)

if __name__ == "__main__":
    main()
