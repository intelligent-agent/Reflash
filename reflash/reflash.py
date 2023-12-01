import os.path
import concurrent.futures
import subprocess
import sqlite3
import threading

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
        self.download_log = ""
        self.install_state = "IDLE"
        self.install_progress = 0.0
        self.install_start_time = 0
        self.install_filename = ""
        self.install_error = ""
        self.install_log = ""
        self.install_bytes_now = 0
        self.install_bytes_total = 0.0
        self.backup_state = "IDLE"
        self.backup_progress = 0.0
        self.backup_filename = ""
        self.backup_total = 0
        self.backup_start_time = 0
        self.backup_log = ""
        self.upload_state = "IDLE"
        self.upload_filename = ""
        self.upload_bytes_total = 1
        self.upload_bytes_now = 0
        self.upload_start_time = 0
        self.upload_log = ""

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
            'download_log',
            'install_state',
            'install_progress',
            'install_start_time',
            'install_filename',
            'install_error',
            'install_log',
            'install_bytes_total',
            'install_bytes_now',
            'backup_state',
            'backup_progress',
            'backup_filename',
            'backup_total',
            'backup_start_time',
            'backup_log',
            'upload_state',
            'upload_filename',
            'upload_bytes_total',
            'upload_bytes_now',
            'upload_start_time',
            'upload_log',
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
            pass
        finally:
            self.lock.release()

class Reflash:
    def __init__(self, settings):
        self.reflash_version_file = settings.get("version_file")
        self.images_folder = settings.get("images_folder")
        self.executor = concurrent.futures.ThreadPoolExecutor(max_workers=4)
        self.sudo = "sudo" if settings.get("use_sudo") else ""
        self.state = State(settings.get("db_file"))
        if not self.state.db_exists():
            self.state.create_default()
        self.state.load()

    def get_version(self):
        return self._run_system_command(f"cat {self.reflash_version_file}")

    def get_emmc_version(self):
        return self._run_system_command(f"{self.sudo} /usr/local/bin/get-emmc-version")

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
        return"IDLE"

    def check_file_integrity(self, filename):
        path = self.images_folder + "/"+filename +".img.xz"
        ret = os.system(f"xz -l {path}")
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
        self.state.download_log = "Starting download\n"
        self.state.download_log += f"Filename: {filename}\n"
        self.state.download_log += f"Filesize: {size}\n"
        self.state.save()
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
        self.state.download_log += "Download finished\n"
        self.state.save()

    def cancel_download(self):
        if self.state.download_state != "DOWNLOADING":
            return False
        try:
            os.remove(self.images_folder + "/" + self.state.download_filename)
        except FileNotFoundError:
            pass
        self.state.download_state = "CANCELLED"
        self.state.download_log += "Download cancelled\n"
        self.state.save()
        return True
    
    def get_download_progress(self):
        ret = {
            "progress": (self.state.download_bytes_now / self.state.download_bytes_total)*100,
            "state": self.state.download_state,
            "start_time": self.state.download_start_time,
            "filename": self.state.download_filename,
            "log": self.state.download_log,
        }
        if self.state.download_state in ["FINISHED", "CANCELLED"]:
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
        self.state.upload_log = "Upload starting\n"
        self.state.upload_log += f"Filename: {filename}\n"
        self.state.upload_log += f"Filesize: {size}\n"
        self.state.upload_filename = filename
        self.state.upload_bytes_total = size
        self.state.upload_bytes_now = 0
        self.state.upload_start_time = start_time
        self.state.save()
        return True

    def upload_finish(self):
        self.state.upload_state = "FINISHED"
        self.state.upload_log += "Upload finished\n"
        self.state.save()
        return True

    def upload_cancel(self):
        self.state.upload_state = "CANCELLED"
        self.state.upload_log += "Upload cancelled\n"
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
            "start_time": self.state.upload_start_time,
            "log": self.state.upload_log,
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
        self._run_system_command(cmd)
        if self.state.install_state != "CANCELLED":
            self.state.install_state = "FINISHED"
            self.state.save()
            self._run_install_finished_commands()

    def get_install_progress(self):
        tr = self._run_system_command("tail -1 /tmp/recore-flash-progress")
        ti = self._run_system_command("cat /tmp/recore-flash-log")
        self.state.install_progress = self._parse_float(tr)
        self.state.install_log = ti
        self.state.save('install_progress')
        self.state.save('install_log')

        ret = {
            "state": self.state.install_state,
            "progress": self.state.install_progress,
            "filename": self.state.install_filename,
            "error": self.state.install_error,
            "start_time": self.state.install_start_time,
            "log": self.state.install_log
        }
        if self.state.install_state not in ["INSTALLING", "IDLE"]:
            self.state.install_state = "IDLE"
            self.state.save('install_state')
        return ret

    def cancel_installation(self):
        self._run_system_command(self.sudo+" pkill -f xz -9")
        self.state.install_state = "CANCELLED"
        self.state.save()
        return True

    def backup_refactor(self, filename, start_time):
        outfile = self.images_folder + "/" + filename
        self.state.backup_filename = filename
        self.state.backup_progress = 0
        self.state.backup_state = "INSTALLING"
        self.state.backup_log = ""
        self.state.backup_start_time = start_time
        self.state.save()
        self.executor.submit(self._ex_backup_refactor, outfile)
        return True

    def _ex_backup_refactor(self, filename):
        cmd = " ".join([self.sudo, "/usr/local/bin/backup-emmc", filename])
        self._run_system_command(cmd)
        if self.state.backup_state != "CANCELLED":
            self.state.backup_state = "FINISHED"
            self.state.save()

    def _parse_float(self, val):
        try:
            val = float(val)
        except:
            val = 0.0
        return val

    def get_backup_progress(self):
        tr = self._run_system_command("tail -1 /tmp/recore-flash-progress")
        ti = self._run_system_command("cat /tmp/recore-flash-log")
        self.state.backup_progress = self._parse_float(tr)
        self.state.backup_log = ti
        self.state.save('backup_progress')
        self.state.save('backup_log')

        ret = {
            "state": self.state.backup_state,
            "progress": self.state.backup_progress,
            "filename": self.state.backup_filename,
            "start_time": self.state.backup_start_time,
            "log": self.state.backup_log,
        }
        if self.state.backup_state == "FINISHED":
            self.state.backup_state = "IDLE"
            self.state.save('backup_state')
        return ret

    def cancel_backup(self):
        self._run_system_command(self.sudo+" pkill -f backup-emmc -9")
        self.state.backup_state = "CANCELLED"
        self.state.save()
        return True

    def save_options(self, options):
        if 'darkmode' in options:
            self.state.settings_darkmode = options['darkmode']
        if 'rebootWhenDone' in options:
            self.state.settings_reboot_when_done = options['rebootWhenDone']
        if 'enableSsh' in options:
            self.state.settings_enable_ssh = options['enableSsh']
        if 'screenRotation' in options:
            self.state.settings_screen_rotation = options['screenRotation']
            self.rotate_screen(options['screenRotation'], "FBCON")
        self.state.save()
        return True

    def get_options(self):
        options = {
            'darkmode': self.state.settings_darkmode,
            'rebootWhenDone': self.state.settings_reboot_when_done,
            'enableSsh': self.state.settings_enable_ssh,
            'screenRotation': self.state.settings_screen_rotation,
        }
        return options

    def _run_install_finished_commands(self):
        self.rotate_screen(self.state.settings_screen_rotation, "CMDLINE")
        self.rotate_screen(self.state.settings_screen_rotation, "XORG")
        self.rotate_screen(self.state.settings_screen_rotation, "WESTON")

        if(self.state.settings_enable_ssh):
            self.enable_ssh()

    def _run_system_command(self, command):
        return subprocess.run(command.split(),
                              capture_output=True,
                              text=True).stdout.strip()

    def enable_ssh(self):
        return self._run_system_command(self.sudo+" /usr/local/bin/enable-emmc-ssh")

    def rotate_screen(self, rotation, place):
        print(f"rotate screen {rotation} {place}")
        return self._run_system_command(f"{self.sudo} /usr/local/bin/rotate-screen {rotation} {place}")

    def reboot(self):
        return self._run_system_command(self.sudo+" /usr/local/bin/reboot-board")

    def shutdown(self):
        return self._run_system_command(self.sudo+" /usr/local/bin/shutdown-board")

    def set_boot_media(self, media):
        return self._run_system_command(f"{self.sudo} /usr/local/bin/set-boot-media {media}")
