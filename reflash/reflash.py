import os.path
import os
import concurrent.futures
import subprocess
import sqlite3
import threading
import queue

class State(object):
    def __init__(self, db_file):
        self.db = sqlite3.connect(db_file, check_same_thread=False)
        self.db.row_factory = sqlite3.Row
        self.cur = self.db.cursor()
        self.lock = threading.Lock()

        self.settings_darkmode = True
        self.settings_reboot_when_done = False
        self.settings_enable_ssh = False
        self.settings_screen_rotation = 0
        self.download_state = "IDLE"
        self.download_bytes_now = 0
        self.download_bytes_total = 1
        self.download_start_time = 0
        self.download_filename = ""
        self.install_state = "IDLE"
        self.install_progress = 0.0
        self.install_start_time = 0
        self.install_filename = ""
        self.install_error = ""
        self.install_bytes_now = 0
        self.install_bytes_total = 0.0
        self.backup_state = "IDLE"
        self.backup_progress = 0.0
        self.backup_filename = ""
        self.backup_total = 0
        self.backup_start_time = 0
        self.upload_state = "IDLE"
        self.upload_filename = ""
        self.upload_bytes_total = 1
        self.upload_bytes_now = 0
        self.upload_start_time = 0

        self.fields = [
            'settings_darkmode',
            'settings_reboot_when_done',
            'settings_enable_ssh',
            'settings_screen_rotation',
            'download_bytes_now',
            'download_bytes_total',
            'download_state',
            'download_start_time',
            'download_filename',
            'install_state',
            'install_progress',
            'install_start_time',
            'install_filename',
            'install_error',
            'install_bytes_total',
            'install_bytes_now',
            'backup_state',
            'backup_progress',
            'backup_filename',
            'backup_total',
            'backup_start_time',
            'upload_state',
            'upload_filename',
            'upload_bytes_total',
            'upload_bytes_now',
            'upload_start_time',
        ]

    def db_exists(self):
        self.cur.execute("SELECT count(name) FROM sqlite_master WHERE type='table' AND name='state'")
        res = dict(self.cur.fetchone())
        return res["count(name)"] == 1

    def create_default(self):
        self.cur.execute("DROP TABLE IF EXISTS state")
        self.cur.execute("CREATE TABLE state(name, value, type)")
        for line in self.fields:
            ins = [line, str(getattr(self, line)), type(getattr(self, line)).__name__]
            self.cur.execute("INSERT INTO state VALUES(?,?,?)", ins)
        self.db.commit()

    def assign_var(self, name, value, type_name):
        var = __builtins__[type_name](value)
        if type_name == 'bool':
            var = (value == "True")
        setattr(self, name, var)

    def load(self):
        self.cur.execute("select * from state")
        res = [dict(row) for row in self.cur.fetchall()]
        for line in res:
            self.assign_var(line["name"], line["value"], line["type"])

    def save(self, field=None):
        fields = self.fields if field == None else [field]
        try:
            self.lock.acquire(True)
            for line in fields:
                value = getattr(self, line)
                ins = f"UPDATE state set value = '{value}' where name = '{line}'"
                self.cur.execute(ins)
            self.db.commit()
        except sqlite3.OperationalError:
            return 1
        finally:
            self.lock.release()
        return 0

class Reflash:
    def __init__(self, settings):
        self.reflash_version_file = settings.get("version_file")
        self.images_folder = settings.get("images_folder")
        self.executor = concurrent.futures.ThreadPoolExecutor(max_workers=4)
        self.sudo = "sudo" if settings.get("use_sudo") else ""
        self.state = State(settings.get("db_file"))
        self.platform = "-d dev" if settings.get("platform") == "dev" else ""
        if not self.state.db_exists():
            self.state.create_default()
        self.state.load()
        self.listeners = []

    def get_state(self):
        self.state.load()
        if self.state.install_state == "INSTALLING":
            return "INSTALLING"
        if self.state.download_state == "DOWNLOADING":
            return "DOWNLOADING"
        if self.state.backup_state == "INSTALLING":
            return "BACKING_UP"
        if self.state.upload_state == "UPLOADING":
            return "UPLOADING"
        return "IDLE"

    def check_file_integrity(self, filename):
        path = self.images_folder + "/"+filename +".img.xz"
        ret = os.system(f"xz -l {path}")
        self._log(f"File check intergity: {'OK' if (ret == 0) else 'Not OK'}")
        return {
            "is_file_ok": (ret == 0)
        }

    def get_local_releases(self):
        import glob
        files = glob.glob(self.images_folder + "/*.img.xz")
        images = [ {
                "name": os.path.basename(f).replace(".img.xz", ""),
                "size": os.stat(f).st_size,
                "id": 0
            } for f in files
        ]
        return images

    def download_refactor(self, url, size, filename, start_time):
        url = url
        self.state.download_bytes_now = 0
        self.state.download_start_time = start_time
        self.state.download_state = "DOWNLOADING"
        self.state.download_bytes_total = size
        self.state.download_filename = filename
        self.state.save()
        self._log(f"starting download of {filename} width size {size}")
        self.executor.submit(self._ex_download_refactor, url, filename)
        return True

    def _ex_download_refactor(self, url, filename):
        import requests
        # Answer by Dennis Patterson
        # https://stackoverflow.com/questions/53101597/how-to-download-binary-file-using-requests
        #
        local_filename = self.images_folder + "/" + filename
        r = requests.get(url, stream=True)
        with open(local_filename, 'wb') as f:
            for chunk in r.iter_content(chunk_size=5*1024*1024):
                if chunk:
                    f.write(chunk)
                    self.state.download_bytes_now += len(chunk)
                    self.state.save('download_bytes_now')
                    if self.state.download_state == "CANCELLED":
                        return
        self.state.is_download_finished = True
        self.state.download_state = "FINISHED"
        self.state.save()

    def cancel_download(self):
        if self.state.download_state != "DOWNLOADING":
            return False
        try:
            os.remove(self.images_folder + "/" + self.state.download_filename)
        except FileNotFoundError:
            pass
        self.state.download_state = "CANCELLED"
        self.state.save()
        return True
    
    def get_download_progress(self):
        ret = {
            "progress": (self.state.download_bytes_now / self.state.download_bytes_total)*100,
            "state": self.state.download_state,
            "start_time": self.state.download_start_time,
            "filename": self.state.download_filename,
        }
        if self.state.download_state in ["FINISHED", "CANCELLED"]:
            if self.state.download_state == "FINISHED":
                self._log("Download finished")
            if self.state.download_state == "CANCELLED":
                self._log("Download cancelled")
            self.state.download_state = "IDLE"
            self.state.save()
        return ret

    def upload_start(self, filename, size, start_time):
        if self.get_state() != "IDLE":
            return False
        if not filename.endswith(".img.xz"):
            return False
        path = self.images_folder+"/"+filename
        open(path, 'w').close()
        self.state.upload_state = "UPLOADING"
        self._log(f"Starting upload of {filename}, size {size}")
        self.state.upload_filename = filename
        self.state.upload_bytes_total = size
        self.state.upload_bytes_now = 0
        self.state.upload_start_time = start_time
        self.state.save()
        return True

    def upload_finish(self):
        self.state.upload_state = "FINISHED"
        self._log("File upload finished")
        self.state.save()
        return True

    def upload_cancel(self):
        self.state.upload_state = "CANCELLED"
        self._log("File upload cancelled")
        self.state.save()
        return True

    def upload_chunk(self, chunk):
        path = self.images_folder+"/"+self.state.upload_filename
        with open(path, "ab+") as f:
            f.write(chunk)
        self.state.upload_bytes_now += len(chunk)
        self.state.save('upload_bytes_now')
        return True

    def get_upload_progress(self):
        ret = {
            "state": self.state.upload_state,
            "progress": (self.state.upload_bytes_now/self.state.upload_bytes_total)*100,
            "filename": self.state.upload_filename,
            "bytes_total": self.state.upload_bytes_total,
            "bytes_now": self.state.upload_bytes_now,
            "start_time": self.state.upload_start_time
        }
        if self.state.upload_state not in ["UPLOADING", "IDLE"]:
            self.state.upload_state = "IDLE"
            self.state.save()
        return ret

    def install_refactor(self, filename, start_time):
        infile = self.images_folder + "/" + filename+".img.xz"
        if not os.path.isfile(infile):
            self.state.install_error = "Chosen file is not present"
            self.state.install_state = "ERROR"
            self.state.save()
            return False
        self.state.install_start_time = start_time
        self.state.install_filename = filename
        self.state.install_progress = 0
        self.state.install_bytes_now = 0
        self.state.install_state = "INSTALLING"
        self.state.install_cancelled = False
        self.state.save()
        self.executor.submit(self._ex_install_refactor, infile)
        return True

    def _ex_install_refactor(self, filename):
        cmd = " ".join([self.sudo, "/usr/local/bin/flash-recore", filename])
        self._system_command_status(cmd)
        if self.state.install_state != "CANCELLED":
            self.state.install_state = "FINISHED"
            self.state.save()
            self._run_install_finished_commands()

    def get_install_progress(self):
        tr = self._system_command_text("tail -1 /tmp/recore-flash-progress")
        self.state.install_progress = self._parse_float(tr)
        self.state.save('install_progress')

        ret = {
            "state": self.state.install_state,
            "progress": self.state.install_progress,
            "filename": self.state.install_filename,
            "error": self.state.install_error,
            "start_time": self.state.install_start_time
        }
        if self.state.install_state not in ["INSTALLING", "IDLE"]:
            self.state.install_state = "IDLE"
            self.state.save('install_state')
        return ret

    def cancel_installation(self):
        self._system_command_text(self.sudo+" pkill -f xz -9")
        self.state.install_state = "CANCELLED"
        self.state.save()
        return True

    def backup_refactor(self, filename, start_time):
        outfile = self.images_folder + "/" + filename
        self.state.backup_filename = filename
        self.state.backup_progress = 0
        self.state.backup_state = "INSTALLING"
        self.state.backup_start_time = start_time
        self.state.save()
        self.executor.submit(self._ex_backup_refactor, outfile)
        return True

    def _ex_backup_refactor(self, filename):
        cmd = " ".join([self.sudo, "/usr/local/bin/backup-emmc", filename])
        result = self._system_command_status(cmd)
        if self.state.backup_state != "CANCELLED":
            self.state.backup_state = "FINISHED"
        if result.status != 0:
            self.state.backup_state == "ERROR"            
        self.state.save('backup_state')

    def _parse_float(self, val):
        try:
            val = float(val)
        except:
            val = 0.0
        return val

    def get_backup_progress(self):
        tr = self._system_command_text("tail -1 /tmp/recore-flash-progress")
        self.state.backup_progress = self._parse_float(tr)
        self.state.save('backup_progress')

        ret = {
            "state": self.state.backup_state,
            "progress": self.state.backup_progress,
            "filename": self.state.backup_filename,
            "start_time": self.state.backup_start_time
        }
        if self.state.backup_state == "FINISHED":
            self.state.backup_state = "IDLE"
            self.state.save('backup_state')
        return ret

    def cancel_backup(self):
        self._system_command_text(self.sudo+" pkill -f backup-emmc -9")
        self.state.backup_state = "CANCELLED"
        return self.state.save()

    def save_options(self, options):
        if 'darkmode' in options:
            self.state.settings_darkmode = options['darkmode']
        if 'rebootWhenDone' in options:
            self.state.settings_reboot_when_done = options['rebootWhenDone']
        if 'enableSsh' in options:
            self.state.settings_enable_ssh = options['enableSsh']
        if 'screenRotation' in options:
            self.state.settings_screen_rotation = options['screenRotation']
        res = self.state.save()
        return {"status": res}

    def get_options(self):
        options = {
            'darkmode': self.state.settings_darkmode,
            'rebootWhenDone': self.state.settings_reboot_when_done,
            'enableSsh': self.state.settings_enable_ssh,
            'screenRotation': self.state.settings_screen_rotation,
            'bootFromEmmc': self.get_boot_media() == "emmc"
        }
        return options

    def _run_install_finished_commands(self):
        self.rotate_screen(self.state.settings_screen_rotation, "CMDLINE")
        self.rotate_screen(self.state.settings_screen_rotation, "XORG")
        self.rotate_screen(self.state.settings_screen_rotation, "WESTON")

        if(self.state.settings_enable_ssh):
            self.set_ssh_enabled("true")

    def _system_command_text(self, command):
        return subprocess.run(command.split(),
                              capture_output=True,
                              text=True).stdout.strip()

    def _system_command_status(self, command):
        res = subprocess.run(command.split(), capture_output=True, text=True)
        return {
            "status": res.returncode,
            "result": res.stdout
        }

    def set_ssh_enabled(self, is_enabled):
        return self._system_command_status(f"{self.sudo} /usr/local/bin/set-ssh-enabled {self.platform} {'true' if is_enabled else 'false'} ")

    def rotate_screen(self, rotation, place):
        return self._system_command_status(f"{self.sudo} /usr/local/bin/rotate-screen {rotation} {place}")

    def set_boot_media(self, media):
        if media not in ["emmc", "usb"]:
            return {"status": 1, "result": f"Unknown media '{media}'"}
        return self._system_command_status(f"{self.sudo} /usr/local/bin/set-boot-media {media}")

    def get_boot_media(self):
        return self._system_command_text(f"{self.sudo} /usr/local/bin/get-boot-media")

    def clear_log(self):
        stat = os.system("echo '-- Log start --' > /var/log/reflash.log")
        return {"status": stat}

    def _log(self, msg):
        os.system(f"echo '[info] {msg}' >> /var/log/reflash.log")

    def get_log(self):
        return self._system_command_text("cat /var/log/reflash.log")

    def start_log_worker(self):
        self.executor.submit(self._ex_log_worker)

    def _ex_log_worker(self):
        cmd = ['tail', '-f', '-n','0', '/var/log/reflash.log']
        p = subprocess.Popen(cmd,  stdout=subprocess.PIPE)
        while True: 
            line = p.stdout.readline().strip().decode("utf-8") 
            self._add_log(line)

    def log_listen(self):
        q = queue.Queue(maxsize=100)
        self.listeners.append(q)
        return q

    def _add_log(self, msg):
        for i in reversed(range(len(self.listeners))):
            try:
                self.listeners[i].put_nowait(f'data: {msg}\n\n')
            except queue.Full:
                print("queue full")
                del self.listeners[i]

    def reboot(self):
        return self._system_command_text(self.sudo+" /usr/local/bin/reboot-board")

    def shutdown(self):
        return self._system_command_text(self.sudo+" /usr/local/bin/shutdown-board")

    def get_version(self):
        return self._system_command_text(f"cat {self.reflash_version_file}")

    def get_emmc_version(self):
        return self._system_command_text(f"{self.sudo} /usr/local/bin/get-emmc-version")

    def get_recore_revision(self):
        return self._system_command_text(f"{self.sudo} /usr/local/bin/get-recore-revision")
