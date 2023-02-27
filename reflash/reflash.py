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

        self.bytes_downloaded = 0
        self.bytes_to_download = 1
        self.download_cancelled = False
        self.download_state = "NOT_STARTED"
        self.is_download_finished = False
        self.install_state = "NOT_STARTED"
        self.install_progress = 0
        self.is_install_finished = False
        self.install_error = ""
        self.install_cancelled = False
        self.backup_progress = 0
        self.is_backup_finished = False
        self.backup_state = "NOT_STARTED"
        self.backup_cancelled = False
        self.backup_error = ""
        self.progress_checker_finished = False
        self.progress_checker_result = ""

        self.fields = [
            'bytes_downloaded',
            'bytes_to_download',
            'download_cancelled',
            'download_state',
            'is_download_finished',
            'install_state',
            'install_progress',
            'is_install_finished',
            'install_error',
            'install_cancelled',
            'backup_progress',
            'is_backup_finished',
            'backup_state',
            'backup_cancelled',
            'backup_error'
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

    def start_check_file_integrity(self, filename):
        path = self.images_folder + "/"+filename +".img.xz"
        self.state.progress_checker_finished = False
        self.state.progress_checker_result = ""
        self.state.save()
        self.executor.submit(self.file_checker_runner, path)
        return True

    def update_check_file_integrity(self):
        return {
            "is_finished": self.state.progress_checker_finished,
            "is_file_ok": self.state.progress_checker_result
        }

    def file_checker_runner(self, path):
        ret = os.system(f"xz -l {path}")
        self.state.progress_checker_result = (ret == 0)
        self.state.progress_checker_finished = True
        self.state.save()

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

    def download_version(self, refactor_image):
        url = refactor_image["url"]
        self.state.bytes_downloaded = 0
        self.state.download_state = "DOWNLOADING"
        self.state.download_cancelled = False
        self.state.is_download_finished = False
        self.state.bytes_to_download = refactor_image["size"]
        self.state.save()
        filename = refactor_image["name"]
        self.executor.submit(self.download_refactor, url, filename)

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
            for chunk in r.iter_content(chunk_size=1024*1024):
                if chunk:
                    f.write(chunk)
                    self.state.bytes_downloaded += len(chunk)
                    self.state.save('bytes_downloaded')
                    if self.state.download_cancelled:
                        return
        self.state.is_download_finished = True
        self.state.download_state = "FINISHED"
        self.state.save()

    def cancel_download(self, refactor_image):
        self.state.download_cancelled = True
        self.state.download_state = "CANCELLED"
        self.state.save()
        filename = refactor_image["name"]
        os.remove(self.images_folder + "/" + filename)

    def get_download_progress(self):
        return {
            "progress": (self.state.bytes_downloaded / self.state.bytes_to_download),
            "state": self.state.download_state,
            "cancelled": self.state.download_cancelled,
            "is_finished": self.state.is_download_finished
        }

    def install_version(self, filename):
        infile = self.images_folder + "/" + filename+".img.xz"
        if not os.path.isfile(infile):
            self.state.install_error = "Chosen file is not present"
            self.state.install_state = "ERROR"
            self.state.save()
            return False
        self.state.install_progress = 0
        self.state.is_install_finished = False
        self.state.bytes_transferred = 0
        self.state.install_state = "INSTALLING"
        self.state.install_cancelled = False
        self.state.bytes_total = self.get_uncompressed_size(infile)
        self.state.save()
        ex = self.executor.submit(self.install_refactor, infile)
        return True

    def get_uncompressed_size(self, infile):
        line = subprocess.run(f"xz -l {infile} | grep MiB",
                              shell=True,
                              capture_output=True,
                              text=True).stdout
        try:
            size = float(line.split()[4].replace(",", "")) * 1024 * 1024
        except:
            self.state.install_error = "Unable to get uncompressed file size"
            self.state.save()
            size = 1
        return size

    def install_refactor(self, filename):
        cmd = ["sudo", "/usr/local/bin/flash-recore", filename]
        self.state.bytes_transferred = 0
        self.state.save()
        self.process = subprocess.Popen(cmd, stdout=subprocess.PIPE, close_fds=True)
        while True:
            time.sleep(0.3)
            if self.process.poll() == 0:
                self.state.is_install_finished = True
                self.state.install_state = "FINISHED"
                self.state.save()
                break
            with open("/tmp/recore-flash-progress") as f:
                lines = f.readlines()
                if len(lines):
                    try:
                        self.state.bytes_transferred = int(lines[-1].strip())
                        self.state.save()
                    except:
                        pass
            if self.state.install_cancelled:
                break
            self.state.install_progress = self.state.bytes_transferred / self.state.bytes_total
            self.state.save()

    def get_install_progress(self):
        return {
            "progress": self.state.install_progress,
            "is_finished": self.state.is_install_finished,
            "error": self.state.install_error,
            "state": self.state.install_state,
            "cancelled": self.state.install_cancelled
        }

    def cancel_installation(self):
        self.state.install_cancelled = True
        self.state.install_state = "CANCELLED"
        self.state.save()
        return True

    def backup_refactor(self, filename):
        outfile = self.images_folder + "/" + filename
        self.state.backup_progress = 0
        self.state.is_backup_finished = False
        self.state.backup_state = "INSTALLING"
        self.state.backup_cancelled = False
        self.state.backup_total = self.get_backup_size()
        ex = self.executor.submit(self.ex_backup_refactor, outfile)
        self.state.save()
        return True

    def get_backup_progress(self):
        return {
            "progress": self.state.backup_progress,
            "is_finished": self.state.is_backup_finished,
            "error": self.state.backup_error,
            "state": self.state.backup_state,
            "cancelled": self.state.backup_cancelled
        }

    def cancel_backup(self):
        self.state.backup_cancelled = True
        self.state.backup_state = "CANCELLED"
        self.state.save()
        return True

    def get_backup_size(self):
        return 100

    def ex_backup_refactor(self, filename):
        cmd = ["sudo", "/usr/local/bin/copy-emmc", filename]
        self.state.backup_transferred = 0
        self.backup_process = subprocess.Popen(cmd, stdout=subprocess.PIPE, close_fds=True)
        while True:
            time.sleep(0.3)
            if self.backup_process.poll() == 0:
                self.state.is_backup_finished = True
                self.state.backup_state = "FINISHED"
                self.state.save()
                break
            with open("/tmp/recore-flash-progress") as f:
                lines = f.readlines()
                if len(lines):
                    try:
                        self.state.backup_transferred = int(lines[-1].strip())
                        self.state.save()
                    except:
                        pass
            if self.state.backup_cancelled:
                break
            self.state.backup_progress = self.state.backup_transferred / self.state.backup_total
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

    def enable_ssh():
        return Reflash.run_system_command("sudo /usr/local/bin/enable-emmc-ssh")

    def reboot():
        return Reflash.run_system_command("sudo /usr/local/bin/reboot-board")

    def set_boot_media(self, media):
        return os.system(f"sudo /usr/local/bin/set-boot-media {media}")
