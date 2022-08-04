import os.path
import concurrent.futures
import subprocess
import time


class Reflash:
    def __init__(self, settings):
        self.refactor_version_file = settings.get("version_file")
        self.images_folder = settings.get("images_folder")
        self.settings_folder = settings.get("settings_folder")
        self.executor = concurrent.futures.ThreadPoolExecutor(max_workers=2)
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

    def get_refactor_version(self):
        with open(self.refactor_version_file, "r") as f:
            version = f.read().replace("\n", "")
            return version

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
        self.bytes_downloaded = 0
        self.download_state = "DOWNLOADING"
        self.download_cancelled = False
        self.is_download_finished = False
        self.bytes_to_download = refactor_image["size"]
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
            for chunk in r.iter_content(chunk_size=1024):
                if chunk:
                    f.write(chunk)
                    self.bytes_downloaded += len(chunk)
                    if self.download_cancelled:
                        return
        self.is_download_finished = True
        self.download_state = "FINISHED"

    def cancel_download(self):
        self.download_cancelled = True
        self.download_state = "CANCELLED"

    def get_download_progress(self):
        return {
            "progress": (self.bytes_downloaded / self.bytes_to_download),
            "state": self.download_state,
            "cancelled": self.download_cancelled,
            "is_finished": self.is_download_finished
        }

    def install_version(self, filename):
        infile = self.images_folder + "/" + filename+".img.xz"
        if not os.path.isfile(infile):
            self.install_error = "Chosen file is not present"
            self.install_state = "ERROR"
            return False
        self.install_progress = 0
        self.is_install_finished = False
        self.bytes_transferred = 0
        self.install_state = "INSTALLING"
        self.install_cancelled = False
        self.bytes_total = self.get_uncompressed_size(infile)
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
            self.install_error = "Unable to get uncompressed file size"
            size = 1
        return size

    def install_refactor(self, filename):
        cmd = ["sudo", "/usr/local/bin/flash-recore", filename]
        self.bytes_transferred = 0
        self.process = subprocess.Popen(cmd, stdout=subprocess.PIPE, close_fds=True)
        while True:
            time.sleep(0.3)
            if self.process.poll() == 0:
                self.is_install_finished = True
                self.install_state = "FINISHED"
                break
            with open("/tmp/recore-flash-progress") as f:
                lines = f.readlines()
                if len(lines):
                    try:
                        self.bytes_transferred = int(lines[-1].strip())
                    except:
                        pass
            if self.install_cancelled:
                break
            self.install_progress = self.bytes_transferred / self.bytes_total

    def get_install_progress(self):
        return {
            "progress": self.install_progress,
            "is_finished": self.is_install_finished,
            "error": self.install_error,
            "state": self.install_state,
            "cancelled": self.install_cancelled
        }

    def cancel_installation(self):
        self.install_cancelled = True
        self.install_state = "CANCELLED"
        return True

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
            return "{}"
        with open(path, "r") as f:
            return json.load(f)

    def enable_ssh():
        return Reflash.run_system_command("sudo /usr/local/bin/enable-emmc-ssh")

    def reboot():
        return Reflash.run_system_command("sudo /usr/local/bin/reboot-board")

    def get_boot_media():
        return Reflash.run_system_command("/usr/local/bin/get-boot-media")

    def change_boot_media():
        if Reflash.get_boot_media() == "emmc":
            os.system("sudo /usr/local/bin/set-boot-media usb")
        else:
            os.system("sudo /usr/local/bin/set-boot-media emmc")

    def is_usb_present():
        return Reflash.run_system_command("/usr/local/bin/is-media-present usb") == "yes"

    def is_emmc_present():
        return Reflash.run_system_command("/usr/local/bin/is-media-present emmc") == "yes"
