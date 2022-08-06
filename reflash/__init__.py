#!/usr/bin/python3

import os
import flask
from .reflash import Reflash

settings = {
        "version_file": "/etc/refactor.version",
        "images_folder": "/opt/reflash/images",
        "settings_folder": "/opt/reflash/settings"
    }
app = flask.Flask(__name__,
            static_folder = "./dist/static",
            template_folder = "./dist")

if app.config['ENV'] == "development":
    from flask_cors import CORS
    CORS(app)

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
    if command == "install_refactor":
        filename = flask.request.json.get("filename")
        stat = reflash.install_version(filename)
        return {"success": stat}
    if command == "cancel_download":
        stat = reflash.cancel_download()
        return {"success": stat}
    if command == "cancel_installation":
        stat = reflash.cancel_installation()
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
    if command == "reboot_board":
        stat = Reflash.reboot()
        return {"success": stat}
    if command == "enable_ssh":
        stat = Reflash.enable_ssh()
        return {"success": stat}
    if command == "get_download_progress":
        return reflash.get_download_progress()
    if command == "get_install_progress":
        return reflash.get_install_progress()
    if command == "get_data":
        return {
            "locals": reflash.get_local_releases(),
            "download_progress": reflash.get_download_progress(),
            "install_progress": reflash.get_install_progress()
        }

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
