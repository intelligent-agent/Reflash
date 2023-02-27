#!/usr/bin/python3

import os
import flask
import sqlite3
from .reflash import Reflash

def get_db(db_file):
    db = getattr(flask.g, '_database', None)
    if db is None:
        db = flask.g._database = sqlite3.connect(db_file, check_same_thread=False)
    return db

settings = {
        "version_file": "/etc/reflash.version",
        "images_folder": "/opt/reflash/images",
        "settings_folder": "/opt/reflash/settings"
    }
app = flask.Flask(__name__,
            static_folder = "./dist/static",
            template_folder = "./dist")

if app.config['ENV'] == "development":
    from flask_cors import CORS
    CORS(app)


with app.app_context():
    db_file = settings["settings_folder"]+"/reflash.db"
    settings["db"] = get_db(db_file)
    reflash = Reflash(settings)

@app.route("/api/run_command",methods = ['POST'])
def run_command():
    command = flask.request.json.get("command")
    if command == "set_boot_media":
        media = flask.request.json.get("media")
        Reflash.set_boot_media(media)
        return { "success": True}
    if command == "download_refactor":
        refactor_image = flask.request.json.get("refactor_image")
        reflash.download_version(refactor_image)
        return { "success": True}
    if command == "cancel_download":
        refactor_image = flask.request.json.get("refactor_image")
        stat = reflash.cancel_download(refactor_image)
        return {"success": stat}
    if command == "upload_chunk":
        chunk = flask.request.json.get("chunk")
        filename = flask.request.json.get("filename")
        is_new_file = flask.request.json.get("is_new_file")
        b64data = chunk.split(",")[1]
        from base64 import b64decode
        decoded_chunk = b64decode(b64data)
        stat = reflash.save_file_chunk(decoded_chunk, filename, is_new_file)
        return { "success": stat}

@app.route('/api/start_file_integrity_check', methods = ['PUT'])
def start_check_file_integrity():
    filename = flask.request.json.get("filename")
    stat = reflash.start_check_file_integrity(filename)
    return {"success": stat}

@app.route('/api/update_file_integrity_check')
def update_check_file_integrity():
    return reflash.update_check_file_integrity()

@app.route('/api/cancel_backup', methods = ['PUT'])
def cancel_backup():
    stat = reflash.cancel_backup()
    return {"success": stat}

@app.route('/api/get_backup_progress')
def get_backup_progress():
    return reflash.get_backup_progress()

@app.route('/api/backup_refactor', methods = ['PUT'])
def backup_refactor():
    filename = flask.request.json.get("filename")
    stat = reflash.backup_refactor(filename)
    return {"success": stat}


@app.route('/api/cancel_installation', methods = ['PUT'])
def cancel_installation():
    stat = reflash.cancel_installation()
    return {"success": stat}

@app.route('/api/install_refactor', methods = ['PUT'])
def install_refactor():
    filename = flask.request.json.get("filename")
    stat = reflash.install_version(filename)
    return {"success": stat}

@app.route('/api/get_data')
def get_data():
    return {
        "locals": reflash.get_local_releases(),
        "download_progress": reflash.get_download_progress(),
        "install_progress": reflash.get_install_progress(),
        "reflash_version": reflash.get_reflash_version()
    }

@app.route('/api/get_download_progress')
def get_download_progress():
    return reflash.get_download_progress()

@app.route('/api/get_install_progress')
def get_install_progress():
    return reflash.get_install_progress()

@app.route('/api/reboot_board', methods = ['PUT'])
def reboot_board():
    stat = Reflash.reboot()
    return {"success": stat}

@app.route('/api/enable_ssh', methods = ['PUT'])
def enable_ssh():
    stat = Reflash.enable_ssh()
    return {"success": stat}

@app.route('/api/set_boot_media', methods = ['PUT'])
def set_boot_media():
    media = flask.request.json.get("media")
    ret = reflash.set_boot_media(media)
    return { "success": ret}

@app.route('/api/options')
def get_options():
    return reflash.read_settings()

@app.route('/api/save_options',methods = ['POST'])
def save_options():
    settings = flask.request.json
    reflash.save_settings(settings)
    return { "success": True}

@app.route('/favicon.ico')
def favicon():
    return flask.send_from_directory(os.path.join(app.root_path, 'dist'),
                               'favicon.ico', mimetype='image/vnd.microsoft.icon')

@app.route('/darkmode.css')
def darkmode():
    return flask.send_from_directory(os.path.join(app.root_path, 'dist'),
                               'darkmode.css', mimetype='text/css')

@app.route('/')
def main():
    return flask.render_template('index.html')


@app.teardown_appcontext
def close_connection(exception):
    db = getattr(flask.g, '_database', None)
    if db is not None:
        db.close()
