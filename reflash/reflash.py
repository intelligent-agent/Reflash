import os.path
import os
import concurrent.futures
import subprocess
import queue
import copy
from sqlitedict import SqliteDict

class State(object):
    def __init__(self, db_file):
        self.db = SqliteDict(db_file)
    
    def create_default(self):
        self.db["settings"] = {
            "darkmode": True,
            "rebootWhenDone": False,
            "enableSsh": False,
            "screenRotation": 0
        }
        self.db["process"] = {
            "state": "IDLE",
            "filename": "Unknown",
            "progress": 0.0,
            "start_time": 0,
            "bytes_now": 0,
            "bytes_total": 0, 
            "error": "No error"
        }       
        self.db.commit()

    def set(self, state, filename, start_time, bytes_total): 
        self.db["process"] = {
            "state": state,
            "filename": filename,
            "start_time": start_time,
            "progress": 0,
            "bytes_now": 0,
            "bytes_total": bytes_total,
            "error": "No error"
        }
        self.db.commit()

    def update(self, state):
        process = self.db["process"]
        process["state"] = state
        self.db["process"] = process
        self.db.commit()

    def set_error(self, error_msg):
        process = self.db["process"]
        process["state"] = "ERROR"
        process["error"] = error_msg
        self.db["process"] = process
        self.db.commit()

    def add_bytes(self, bytes):
        process = self.db["process"]
        process["bytes_now"] += bytes
        process["progress"] = (process["bytes_now"]/process["bytes_total"])*100
        self.db["process"] = process
        self.db.commit()

    def set_progress(self, progress):
        process = self.db["process"]
        process["progress"] = progress
        self.db["process"] = process
        self.db.commit()

    def set_setting(self, name, value):
        settings = self.db["settings"]
        if name not in settings:
            return
        settings[name] = value
        self.db["settings"] = settings
        self.db.commit()

    def db_exists(self):
        return len(self.db) > 0

class Reflash:
    def __init__(self, settings):
        self.reflash_version_file = settings.get("version_file")
        self.images_folder = settings.get("images_folder")
        self.executor = concurrent.futures.ThreadPoolExecutor(max_workers=4)
        self.futures = None
        self.sudo = "sudo" if settings.get("use_sudo") else ""
        self.state = State(settings.get("db_file"))
        self.platform = "-d dev" if settings.get("platform") == "dev" else ""
        self.settings = settings
        if not self.state.db_exists():
            self.state.create_default()
        self.listeners = []
        self.log_worker_active = False

    def teardown(self):
        self.state.db.close()

    def get_state(self):
        return self.state.db["process"]["state"]

    def download_refactor(self, url, size, filename, start_time):
        self.state.set("DOWNLOADING", filename, start_time, size)
        self._log(f"starting download of {filename} width size {size}")
        self.futures = self.executor.submit(self._ex_download_refactor, url, filename)
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
                    self.state.add_bytes(len(chunk))
                    if self.get_state() == "CANCELLED":
                        return
        self.state.update("FINISHED")

    def cancel_download(self):
        if self.get_state() != "DOWNLOADING":
            return False
        try:
            os.remove(self.images_folder + "/" + self.state.db["process"]["filename"])
        except FileNotFoundError:
            pass
        self.state.update("CANCELLED")
        return True
    
    def get_download_progress(self):
        state = copy.copy(self.state.db["process"])
        if self.get_state() == "FINISHED":
            self._log("Download finished")
            self.state.update("IDLE")
        if self.get_state() == "CANCELLED":
            self._log("Download cancelled")
            self.state.update("IDLE")
        return state

    def upload_start(self, filename, size, start_time):
        if self.get_state() != "IDLE":
            return False
        if not filename.endswith(".img.xz"):
            return False
        path = self.images_folder+"/"+filename
        open(path, 'w').close()
        self.state.set("UPLOADING", filename, start_time, size)
        self._log(f"Starting upload of {filename}, size {size}")
        return True

    def upload_finish(self):
        self.state.update("FINISHED")
        return True

    def upload_cancel(self):
        self.state.update("CANCELLED")
        return True

    def upload_chunk(self, chunk):
        path = self.images_folder+"/"+self.state.db["process"]["filename"]
        with open(path, "ab+") as f:
            f.write(chunk)
        self.state.add_bytes(len(chunk))
        return True

    def get_upload_progress(self):
        process = copy.copy(self.state.db["process"])
        print(self.get_state())
        if self.get_state() == "FINISHED":
            self._log("Upload finished")
            self.state.update("IDLE")
        if self.get_state() == "CANCELLED":
            self._log("Upload cancelled")
            self.state.update("IDLE")
        return process
    
    def install_refactor(self, filename, start_time):
        infile = self.images_folder + "/" + filename+".img.xz"
        if not os.path.isfile(infile):
            self._log_error("Files does not exist")
            self.state.set_error("File does not exist")
            return False
        self.state.set("INSTALLING", filename, start_time, 0)
        self.executor.submit(self._ex_install_refactor, infile)
        return True

    def _ex_install_refactor(self, filename):
        cmd = " ".join([self.sudo, "/usr/local/bin/flash-recore", filename])
        result = self._system_command_code(cmd)
        if result != 0:
            self.state.set_error("There was an error during install. Check the log for details.")
            return
        if self.get_state() == "INSTALLING":
            self.state.update("FINISHED")
            self._run_install_finished_commands()

    def get_install_progress(self):
        tr = self._system_command_text("tail -1 /tmp/recore-flash-progress")
        self.state.set_progress(self._parse_float(tr))
        state = copy.copy(self.state.db["process"])
        if self.get_state() in ["FINISHED", "ERROR", "CANCELLED"]:
            self.state.update("IDLE")
        return state

    def cancel_installation(self):
        self._system_command_text(self.sudo+" pkill -f xz -9")
        self.state.update("CANCELLED")
        return True

    def backup_refactor(self, filename, start_time):
        outfile = self.images_folder + "/" + filename
        self.state.set("BACKUPING", filename, start_time, 0)
        self.executor.submit(self._ex_backup_refactor, outfile)
        return True

    def _ex_backup_refactor(self, filename):
        cmd = " ".join([self.sudo, "/usr/local/bin/backup-emmc", filename])
        result = self._system_command_code(cmd)
        if self.get_state() != "CANCELLED":
            self.state.update("FINISHED")
        if result != 0:
            self.state.set_error("There was an error during backup. Check the log for details.")

    def _parse_float(self, val):
        try:
            val = float(val)
        except:
            val = 0.0
        return val

    def get_backup_progress(self):
        tr = self._system_command_text("tail -1 /tmp/recore-flash-progress")
        self.state.set_progress(self._parse_float(tr))
        state = copy.copy(self.state.db["process"])
        if self.get_state() == "FINISHED":
            self.state.update("IDLE")
        return state

    def cancel_backup(self):
        self._system_command_text(self.sudo+" pkill -f backup-emmc -9")
        self.state.update("CANCELLED")
        return True

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

    def save_options(self, options):
        for option, value in options.items():
            self.state.set_setting(option, value)
        return {"status": 0}

    def get_options(self):
        options = self.state.db["settings"]
        options['bootFromEmmc'] = self.get_boot_media() == "emmc"
        return options

    def _run_install_finished_commands(self):
        self.rotate_screen(self.state.db["settings"]["screenRotation"], "CMDLINE", "TRUE")
        self.rotate_screen(self.state.db["settings"]["screenRotation"], "XORG", "TRUE")
        self.rotate_screen(self.state.db["settings"]["screenRotation"], "WESTON", "TRUE")

        if(self.state.db["settings"]["enableSsh"]):
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

    def _system_command_code(self, command):
        res = subprocess.run(command.split(), capture_output=True, text=True)
        return res.returncode

    def set_ssh_enabled(self, is_enabled):
        return self._system_command_status(f"{self.sudo} /usr/local/bin/set-ssh-enabled {self.platform} {'true' if is_enabled else 'false'} ")

    def rotate_screen(self, rotation, place, restart_app):
        if rotation not in [0, 90, 180, 270]:
            return {"status": 1, "result": f"Unknown rotation '{rotation}'"}
        if place not in ["FBCON", "CMDLINE", "XORG", "WESTON"]:
            return {"status": 1, "result": f"Unknown app '{place}'"}
        if restart_app not in ["TRUE", "FALSE"]:
            return {"status": 1, "result": f"Unknown restart command '{restart_app}'"}
        return self._system_command_status(f"{self.sudo} /usr/local/bin/rotate-screen {rotation} {place} {restart_app}")

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
    
    def _log_error(self, msg):
        os.system(f"echo '[error] {msg}' >> /var/log/reflash.log")

    def get_log(self):
        return self._system_command_text("cat /var/log/reflash.log")

    def start_log_worker(self):
        self.log_worker_active = True
        self.futures = self.executor.submit(self._ex_log_worker)

    def stop_log_worker(self):
        self.log_worker_active = False
        self.executor.shutdown(wait=False)
        self.teardown()

    def _ex_log_worker(self):
        cmd = ['tail', '-f', '-n','0', '/var/log/reflash.log']
        p = subprocess.Popen(cmd,  stdout=subprocess.PIPE)
        while self.log_worker_active:
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

    def get_usb_version(self):
        return self._system_command_text(f"{self.sudo} /usr/local/bin/get-usb-version")

    def get_recore_revision(self):
        return self._system_command_text(f"{self.sudo} /usr/local/bin/get-recore-revision")

    def is_usb_present(self):
        return True if self._system_command_text(f"{self.sudo} /usr/local/bin/is-usb-present") == "true" else False

    def is_ssh_enabled(self):
        return True if self._system_command_text(f"{self.sudo} /usr/local/bin/is-ssh-enabled") == "true" else False

    def get_available_bytes(self):
        if self.settings.get("platform") == "dev":
            return self._system_command_text(f"{self.sudo} df /dev/nvme0n1p2 --output=avail").split("\n")[1]
        else:
            return self._system_command_text(f"{self.sudo} df /dev/sda2 --output=avail").split("\n")[1]
        
