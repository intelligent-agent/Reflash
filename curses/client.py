#!/usr/bin/python3

import curses
import time
import enum
import logging

import sys
sys.path.append('..')
sys.path.append('.')

import reflash as ref

class ProgressBar(object):
    def __init__(self):
        height, width = stdscr.getmaxyx()
        self.width = int(width/3)
        self.height = 3
        self.x = int((width/2)-(self.width/2))
        self.y = int((height/2)-(self.height/2))
        self.win = curses.newwin(self.height, self.width, self.y, self.x)
        self.colors = curses.color_pair(1)
        self.progress = 0
        self.visible = False

    def set_progress(self, progress):
        self.visible = True
        self.progress = int((self.width-2)*progress/100.0)

    def draw(self):
        bar = '█'
        self.win.erase()
        if self.visible:
            self.win.border(0)
            self.win.addstr(1, 1, bar*self.progress, self.colors)
        self.win.refresh()

    def hide(self):
        self.visible = False

class CenterText(object):
    def __init__(self, y_offset):
        self.text = ""
        height, width = stdscr.getmaxyx()
        self.height = 2
        self.y = int((height/2)-(self.height/2))+y_offset
        self.x = 0
        self.width = width-1
        self.win = curses.newwin(self.height, self.width, self.y, self.x)
        self.visible = True

    def set_text(self, text):
        self.visible = True
        self.text = text

    def draw(self):
        self.win.erase()
        if self.visible:
            self.win.addstr(1, int((self.width/2)-(len(self.text)/2)), self.text, curses.A_NORMAL)
        self.win.refresh()

    def set_offset(self, y_offset):
        self.y = int((height/2)-(self.height/2))+y_offset
        self.win = curses.newwin(self.height, self.width, self.y, self.x)

    def hide(self):
        self.visible = False

class Status(CenterText):
    def __init__(self):
        CenterText.__init__(self, 2)

class Header(CenterText):
    def __init__(self):
        CenterText.__init__(self, -2)
        self.text = "REFLASH"

class State(enum.Enum):
    IDLE = 1
    DOWNLOADING = 2
    INSTALLING = 3

class StateMachine(object):
    def __init__(self):
        self.state = State.IDLE

    def set_state(self, state):
        self.state = state

    def get_state(self):
        return self.state


logger = logging.getLogger(__file__)
hdlr = logging.FileHandler(__file__ + ".log")
logger.addHandler(hdlr)
logger.setLevel(logging.DEBUG)


stdscr = curses.initscr()
curses.curs_set(0)
height, width = stdscr.getmaxyx()

rgb_scale = 3.90625
curses.start_color()
curses.init_pair(1, curses.COLOR_CYAN, curses.COLOR_WHITE)
curses.init_pair(2, curses.COLOR_WHITE, curses.COLOR_BLACK)
curses.init_color(curses.COLOR_CYAN, int(4*rgb_scale), int(163*rgb_scale), int(229*rgb_scale))
curses.init_color(curses.COLOR_WHITE, int(241*rgb_scale), int(241*rgb_scale), int(241*rgb_scale))
curses.init_color(curses.COLOR_BLACK, int(41*rgb_scale), int(42*rgb_scale), int(44*rgb_scale))
stdscr.bkgd(' ', curses.color_pair(2) | curses.A_NORMAL)

progress_bar = ProgressBar()
status = Status()
header = Header()
state = StateMachine()

settings = {
    "version_file": "/etc/reflash.version",
    "images_folder": "/opt/reflash/images",
    "db_file": "/opt/reflash/reflash.db",
    "use_sudo": True,
}

reflash = ref.Reflash(settings)

k = 0
stdscr.nodelay(1)
curses.noecho()
old_global_state = "NONE"
while (k != ord('q')):
    k = stdscr.getch()
    reflash.refresh()
    global_state = reflash.get_state()
    if global_state != old_global_state:
        stdscr.erase()
        stdscr.refresh()
    old_global_state = global_state
    if global_state == 'IDLE':
        progress_bar.hide()
        status.hide()
        header.set_offset(0)
    else:
        status.set_offset(1)
        header.set_offset(-3)

    if global_state == 'DOWNLOADING':
        progress = reflash.get_download_progress()
        status.set_text("Downloading")
        progress_bar.set_progress(progress['progress'])
    elif global_state == 'UPLOADING':
        progress = reflash.get_upload_progress()
        status.set_text("Uploading")
        progress_bar.set_progress(progress['progress'])
    elif global_state == 'INSTALLING':
        progress = reflash.get_install_progress()
        status.set_text("Installing")
        progress_bar.set_progress(progress['progress'])
    elif global_state == 'BACKING_UP':
        progress = reflash.get_backup_progress()
        status.set_text("Backing up")
        progress_bar.set_progress(progress['progress'])
    status.draw()
    progress_bar.draw()
    header.draw()
    curses.doupdate()
    time.sleep(1)

curses.endwin()
