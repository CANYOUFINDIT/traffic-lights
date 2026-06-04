from machine import Pin
from time import sleep_ms, ticks_diff, ticks_ms
import select
import sys


red = Pin(4, Pin.OUT)
yellow = Pin(3, Pin.OUT)
green = Pin(2, Pin.OUT)

pins = {
    "red": red,
    "yellow": yellow,
    "green": green,
}

mode = "blink_green"
blink_on = False
last_blink_ms = ticks_ms()
blink_interval_ms = 400


def all_off():
    red.value(0)
    yellow.value(0)
    green.value(0)


def solid(name):
    all_off()
    pins[name].value(1)


def apply_mode():
    global blink_on, last_blink_ms

    blink_on = False
    last_blink_ms = ticks_ms()
    if mode in pins:
        solid(mode)
    elif mode == "off":
        all_off()
    elif mode.startswith("blink_"):
        all_off()


def handle_command(raw):
    global mode

    cmd = raw.strip().lower()
    if not cmd:
        return
    if cmd in (
        "red",
        "yellow",
        "green",
        "off",
        "blink_red",
        "blink_yellow",
        "blink_green",
    ):
        mode = cmd
        apply_mode()
        print("ok", cmd)
    elif cmd == "status":
        print(mode)
    else:
        print("unknown", cmd)


def update_blink():
    global blink_on, last_blink_ms

    if not mode.startswith("blink_"):
        return
    now = ticks_ms()
    if ticks_diff(now, last_blink_ms) >= blink_interval_ms:
        last_blink_ms = now
        blink_on = not blink_on
        all_off()
        name = mode[6:]
        if blink_on and name in pins:
            pins[name].value(1)


poll = select.poll()
poll.register(sys.stdin, select.POLLIN)
apply_mode()

while True:
    if poll.poll(20):
        handle_command(sys.stdin.readline())
    update_blink()
    sleep_ms(20)
