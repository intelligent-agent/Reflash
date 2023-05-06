import pytest
import sys
import os
import shutil
import requests_mock
import reflash
import time


from unittest.mock import MagicMock, patch
from reflash.reflash import Reflash

@patch("mypackage.proc.subprocess.run")

def stream_callback(req, context):
    return "Y"*10*1024*1024

def create_file_with_size(name, size):
    with open(name, 'w+') as f:
        f.write('0' * size)

def create_dummy_file(name):
    open(name, 'a').close()
    os.system(f"xz -z {name}")

@pytest.fixture()
def r():
    shutil.rmtree('.tmp-test', ignore_errors=True)
    settings = {
        "version_file": ".tmp-test/etc/reflash/reflash.version",
        "images_folder": ".tmp-test/opt/reflash/images",
        "db_file": ".tmp-test/opt/reflash/reflash.db",
        "use_sudo": False
    }
    os.makedirs(".tmp-test/etc/reflash/", exist_ok = True)
    path = os.path.join("./", settings['images_folder'])
    os.makedirs(path, exist_ok = True)
    with open(settings["version_file"], 'w+') as f:
        f.write("v0.1.2\n")

    r = reflash.Reflash(settings)
    yield(r)

@pytest.fixture()
def p():
    progress = {'filename': 'hamburger.img.xz', 'progress': 0.0, 'start_time': 666, 'state': 'DOWNLOADING'}
    yield(progress)

class TestReflash:
    def test_get_state(self, r):
        assert r.get_state() == "IDLE"
    
    def test_check_file_integrity(self, r):
        assert r.check_file_integrity("missing_file") == {'is_file_ok': False}
        create_dummy_file(".tmp-test/opt/reflash/images/pizza.img")
        assert r.check_file_integrity('pizza') == {'is_file_ok': True}

    def test_get_local_releases(self, r):
        assert r.get_local_releases() == []
        create_dummy_file(".tmp-test/opt/reflash/images/hamburger.img")
        assert r.get_local_releases() == [{'id': 0, 'name': 'hamburger', 'size': 32}]
    
    def test_upload_start(self, r):
        filename = "pizza.img.xz"
        size = 42
        start_time = 666
        r.upload_start(filename, size, start_time)
        assert r.get_state() == "UPLOADING"
        assert r.upload_start(filename, size, start_time) == False

    def test_upload_start_repeat(self, r):
        filename = "pizza.img.xz"
        size = 42
        start_time = 666
        assert r.upload_start(filename, size, start_time) == True
        assert r.upload_start(filename, size, start_time) == False

    def test_upload_chunk(self, r):
        size = 42
        start_time = 666
        assert r.upload_start("pizza.img.xz", size, start_time) == True
        assert r.get_state() == "UPLOADING"
        assert r.upload_chunk(bytes("hamburger", 'utf-8')) == True
        assert r.upload_finish() == True
        assert r.get_state() == "IDLE"
        assert r.get_local_releases() == [{'id': 0, 'name': 'pizza', 'size': len("hamburger")}]

    def test_download_refactor_ok(self, r, p):
        with requests_mock.Mocker() as mock:
            url = "http://pizza.com"
            size = 5
            filename = "hamburger.img.xz"
            start_time = 666
            mock.get("http://pizza.com", text='tacos')
            assert r.download_refactor(url, size, filename, start_time) == True
            assert r.get_download_progress() == p
            time.sleep(0.1)
            p['state'] = 'FINISHED'
            p['progress'] = 100.0
            assert r.get_download_progress() == p
            p['state'] = 'IDLE'
            assert r.get_download_progress() == p
            assert r.get_local_releases() == [{'id': 0, 'name': 'hamburger', 'size':5}]

    def test_download_refactor_cancel(self, r, p):
        pass

    def test_install_missing_file(self, r):
        assert r.install_refactor("tacos", 567) == False

    def test_install_ok(self, r):
        create_dummy_file(".tmp-test/opt/reflash/images/tacos.img")
        assert r.install_refactor("tacos", 39) == True
        assert r.get_state() == "INSTALLING"
        assert r.get_install_progress()['state'] == "INSTALLING"

    def test_set_get_optons(self ,r):
        options = {
            "darkmode": True,
            "enableSsh": True,
            "rebootWhenDone": False
        }
        assert r.save_options(options) == True

        # adding other option is silently ignored
        options['pizza'] = True
        assert r.save_options(options) == True
        del options['pizza']
        assert r.get_options() == options

    @patch('subprocess.run')
    def test_backup_refactor(self, run, r):
        filename = "pizza"
        start_time = 123
        create_file_with_size(".tmp/dev/mmcblk0", 100)
        assert r.backup_refactor(filename, start_time) == True
        cmd = ['/usr/local/bin/backup-emmc', '.tmp-test/opt/reflash/images/pizza']
        run.assert_called_once_with(cmd, capture_output=True, text=True)
