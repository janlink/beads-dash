#!/usr/bin/env python3
"""Plays a keystroke tape against bdash in a pseudo-terminal and writes an
asciicast v2 file. Standard library only.

usage: record.py <workspace-dir> <out.cast> [bdash-binary]
"""

import codecs
import fcntl
import json
import os
import pty
import select
import signal
import struct
import sys
import termios
import time

COLS, ROWS = 140, 38

ESC = "\x1b"
ENTER = "\r"
TAB = "\t"
DOWN = ESC + "[B"
UP = ESC + "[A"
RIGHT = ESC + "[C"
LEFT = ESC + "[D"
CTRL_S = "\x13"

# The docked detail panel starts hidden so the views fill the screen; it is
# shown once (D) in the Tree view, and Enter opens the detail as an overlay.
# (seconds to wait before the keys, keys). Keys are typed one by one with TYPE_GAP.
TAPE = [
    (1.5, ""),
    (0.3, "D"),
    (1.0, "2"),
    (1.2, "j"),
    (0.6, "j"),
    (0.6, "j"),
    (1.0, "D"),
    (3.0, "D"),
    (0.8, ENTER),
    (1.8, ESC),
    (0.8, "3"),
    (1.6, "l"),
    (1.0, "j"),
    (1.2, "4"),
    (1.6, "6"),
    (2.0, "2"),
    (0.8, "/"),
    (0.5, "payment"),
    (1.8, ESC),
    (0.8, "space"),
    (0.6, "k"),
    (0.6, "space"),
    (0.8, "x"),
    (3.5, ESC),
    (0.8, "?"),
    (2.0, ESC),
    (0.8, "q"),
]
TYPE_GAP = 0.06


def keys_of(text):
    if text == "space":
        return [" "]
    if text.startswith(ESC) or len(text) == 1:
        return [text]
    return list(text)


def winsize(fd):
    fcntl.ioctl(fd, termios.TIOCSWINSZ, struct.pack("HHHH", ROWS, COLS, 0, 0))


def wait_for(drain, events, needle, limit):
    """Drains output until needle shows up in it; False when the child ends or
    the limit passes first."""
    end = time.monotonic() + limit
    while time.monotonic() < end:
        if not drain(0.2):
            return False
        if needle in "".join(e[2] for e in events):
            return True
    sys.exit("demo/record.py: %r never appeared in the output" % needle)


def main():
    if len(sys.argv) < 3:
        sys.exit(__doc__)
    workspace, out = sys.argv[1], sys.argv[2]
    binary = sys.argv[3] if len(sys.argv) > 3 else "bdash"

    pid, fd = pty.fork()
    if pid == 0:
        env = dict(os.environ, TERM="xterm-256color", COLORTERM="truecolor", COLUMNS=str(COLS), LINES=str(ROWS))
        env.pop("NO_COLOR", None)
        os.execvpe(binary, [binary, "--no-mouse", workspace], env)
    winsize(fd)
    os.kill(pid, signal.SIGWINCH)

    start = time.monotonic()
    events = []
    decoder = codecs.getincrementaldecoder("utf-8")("replace")

    def drain(timeout):
        end = time.monotonic() + timeout
        while True:
            left = end - time.monotonic()
            if left <= 0:
                return True
            ready, _, _ = select.select([fd], [], [], left)
            if not ready:
                return True
            try:
                data = os.read(fd, 65536)
            except OSError:
                return False
            if not data:
                return False
            text = decoder.decode(data)
            if text:
                events.append([round(time.monotonic() - start, 6), "o", text])

    wait_for(drain, events, " live ", 30)
    ready_at = events[-1][0]
    alive = True
    for wait, keys in TAPE:
        alive = drain(wait)
        if not alive:
            break
        for k in keys_of(keys):
            os.write(fd, k.encode())
            alive = drain(TYPE_GAP)
            if not alive:
                break
        if not alive:
            break
    drain(0.5)
    try:
        os.kill(pid, signal.SIGTERM)
    except ProcessLookupError:
        pass
    os.waitpid(pid, 0)

    header = {"version": 2, "width": COLS, "height": ROWS, "timestamp": int(time.time()), "env": {"TERM": "xterm-256color"}}
    with open(out, "w", encoding="utf-8", newline="\n") as f:
        f.write(json.dumps(header) + "\n")
        for e in events:
            e[0] = round(max(e[0] - ready_at, 0.0), 6)
            f.write(json.dumps(e, ensure_ascii=False) + "\n")


if __name__ == "__main__":
    main()
