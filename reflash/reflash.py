import os.path
import concurrent.futures
import subprocess
import time
import sqlite3

class State(object):
    def __init__(self, db):
        self.db = db
        self.db.row_factory = sqlite3.Row
        self.cur = self.db.cursor()

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
        self.install_log = ""
        self.install_bytes_now = 0
        self.install_bytes_total = 0.0
        self.backup_state = "IDLE"
        self.backup_progress = 0.0
        self.backup_filename = ""
        self.backup_total = 0
        self.backup_start_time = 0
        self.backup_log = ""

        self.fields = [
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
            'install_log',
            'install_bytes_total',
            'install_bytes_now',
            'backup_state',
            'backup_progress',
            'backup_filename',
            'backup_total',
            'backup_start_time',
            'backup_log'
            ]

    def db_exists(self):
        self.cur.execute("SELECT count(name) FROM sqlite_master WHERE type='table' AND name='state'")
        res = dict(self.cur.fetchone())
        return res["count(name)"] == 1

    def create_default(self):
        self.cur.execute("DROP TABLE IF EXISTS state")
        self.cur.execute("CREATE TABLE state(name, value, type)")

        for line in self.fields:
            ins = [line, getattr(self, line), type(getattr(self, line)).__name__]
            self.cur.execute("INSERT INTO state VALUES(?,?,?)", ins)
        self.db.commit()


    def assign_var(self, name, value, type_name):
        var = __builtins__[type_name](value)
        setattr(self, name, var)

    def load(self):
        self.cur.execute("select * from state")
        res = [dict(row) for row in self.cur.fetchall()]
        for line in res:
            self.assign_var(line["name"], line["value"], line["type"])

    def save(self, field=None):
        fields = self.fields if field == None else [field]
        for line in fields:
            value = getattr(self, line)
            ins = f"UPDATE state set value = '{value}' where name = '{line}'"
            self.cur.execute(ins)
        self.db.commit()

class Reflash:
    def __init__(self, settings):
        self.reflash_version_file = settings.get("version_file")
        self.images_folder = settings.get("images_folder")
        self.settings_folder = settings.get("settings_folder")
        self.executor = concurrent.futures.ThreadPoolExecutor(max_workers=4)
        self.state = State(settings.get("db"))
        if not self.state.db_exists():
            self.state.create_default()
        self.state.load()

    def get_reflash_version(self):
        with open(self.reflash_version_file, "r") as f:
            version = f.read().replace("\n", "")
            return version

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

    def download_version(self, refactor_image, start_time):
        url = refactor_image["url"]
        self.state.download_bytes_now = 0
        self.state.download_start_time = start_time
        self.state.download_state = "DOWNLOADING"
        self.state.download_bytes_total = refactor_image["size"]
        self.state.download_filename = refactor_image["name"]
        self.state.save()
        self.executor.submit(self.download_refactor, url, refactor_image["name"])

    def save_file_chunk(self, chunk, filename, is_new_file):
        open_rights = "wb+" if is_new_file else "ab+"
        with open(self.images_folder+"/"+filename, open_rights) as f:
            f.write(chunk)
        return True

    def download_refactor(self, url, filename):
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
        os.remove(self.images_folder + "/" + self.state.download_filename)
        self.state.download_state = "CANCELLED"
        self.state.save()

    def get_download_progress(self):
        ret = {
            "progress": (self.state.download_bytes_now / self.state.download_bytes_total)*100,
            "state": self.state.download_state,
            "start_time": self.state.download_start_time,
            "filename": self.state.download_filename,
        }
        if self.state.download_state == "FINISHED":
            self.state.download_state = "IDLE"
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
        ex = self.executor.submit(self.ex_install_refactor, infile)
        return True

    def ex_install_refactor(self, filename):
        cmd = ["sudo", "/usr/local/bin/flash-recore", filename]
        self.process = subprocess.Popen(cmd, stdout=subprocess.PIPE, close_fds=True)
        while True:
            time.sleep(0.3)
            tr = Reflash.run_system_command("tail -1  /tmp/recore-flash-progress")
            ti = Reflash.run_system_command("cat /tmp/recore-flash-log")
            try:
                self.state.install_progress = float(tr.strip())
                self.state.install_log = ti
                self.state.save()
            except:
                pass
            if self.process.poll() == 0:
                self.state.install_state = "FINISHED"
                self.state.save()
                break
            if self.state.install_state == "CANCELLED":
                self.state.install_log += "\nInstallation cancelled"
                break

    def get_install_progress(self):
        ret = {
            "state": self.state.install_state,
            "progress": self.state.install_progress,
            "filename": self.state.install_filename,
            "error": self.state.install_error,
            "start_time": self.state.install_start_time,
            "log": self.state.install_log
        }
        if self.state.install_state == "FINISHED":
            self.state.install_state = "IDLE"
            self.state.save()
        return ret

    def cancel_installation(self):
        Reflash.run_system_command("sudo pkill -f xz -9")
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
        ex = self.executor.submit(self.ex_backup_refactor, outfile)
        return True

    def get_backup_progress(self):
        ret = {
            "state": self.state.backup_state,
            "progress": self.state.backup_progress,
            "filename": self.state.backup_filename,
            "start_time": self.state.backup_start_time,
            "log": self.state.backup_log,
        }
        if self.state.backup_state == "FINISHED":
            self.state.backup_state = "IDLE"
            self.state.save()
        return ret

    def cancel_backup(self):
        Reflash.run_system_command("sudo pkill -f backup-emmc -9")
        self.state.backup_state = "CANCELLED"
        self.state.save()
        return True

    def ex_backup_refactor(self, filename):
        cmd = ["sudo", "/usr/local/bin/backup-emmc", filename]
        self.backup_transferred = 0
        self.backup_process = subprocess.Popen(cmd, stdout=subprocess.PIPE, close_fds=True)
        while True:
            time.sleep(0.3)
            status = self.backup_process.poll()
            tr = Reflash.run_system_command("tail -1 /tmp/recore-flash-progress")
            ti = Reflash.run_system_command("cat /tmp/recore-flash-log")
            try:
                self.state.backup_progress = float(tr.strip())
                self.state.backup_log = ti
                self.state.save()
            except:
                pass
            if status == 0:
                self.state.backup_state = "FINISHED"
                self.state.save()
                break
            if status != None:
                self.state.backup_state = "ERROR"
                self.state.backup_log += f"\nError {status}"
                self.state.save()
                break
            if self.state.backup_state == "CANCELLED":
                break
            self.state.save()

    def run_system_command(command):
        return subprocess.run(command.split(),
                              capture_output=True,
                              text=True).stdout.strip()

    def save_settings(self, settings):
        import json
        json_file = json.dumps(settings, indent=4)
        with open(self.settings_folder+"/settings.json", "w+") as f:
            f.write(json_file)
        return True

    def read_settings(self):
        import json
        path = self.settings_folder+"/settings.json"
        if not os.path.isfile(path):
            return {}
        with open(path, "r") as f:
            try:
                return json.load(f)
            except:
                return {}

    def enable_ssh(self):
        return Reflash.run_system_command("sudo /usr/local/bin/enable-emmc-ssh")

    def reboot(self):
        return Reflash.run_system_command("sudo /usr/local/bin/reboot-board")

    def shutdown(self):
        return Reflash.run_system_command("sudo /usr/local/bin/shutdown-board")

    def set_boot_media(self, media):
        return os.system(f"sudo /usr/local/bin/set-boot-media {media}")
