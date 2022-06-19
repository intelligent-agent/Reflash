#!/usr/bin/python3

import os
import flask
from .reflash import Reflash

def get_settings():
    return {
        "version_file": "/etc/refactor.version",
        "images_folder": "/opt/images/"
    }

app = flask.Flask(__name__)

settings = get_settings()
reflash = Reflash(settings)


@app.route("/run_command",methods = ['POST'])
def run_command():
    command = flask.request.json.get("command")
    if command == "change_boot_media":
        Reflash.change_boot_media()
        status = {
            "boot_media": reflash.get_boot_media()
        }
        return flask.jsonify(**status)
    if command == "download_refactor":
        refactor_image = flask.request.json.get("refactor_image")
        reflash.download_version(refactor_image)
        return { "success": True}
    if command == "install_refactor":
        filename = flask.request.json.get("filename")
        reflash.install_version(filename)
        return {"success": True}
    if command == "cancel_download":
        stat = reflash.cancel_download()
        status = {"success": stat}
        return status
    if command == "get_download_progress":
        progress = reflash.get_download_progress()
        return progress
    if command == "get_install_progress":
        progress = reflash.get_install_progress()
        return progress
    if command == "get_data":
        data = {
            "releases": reflash.get_releases(),
            "locals": reflash.get_local_releases(),
            "boot_media": Reflash.get_boot_media(),
            "usb_present": Reflash.is_usb_present(),
            "emmc_present": Reflash.is_emmc_present(),
            "download_progress": reflash.get_download_progress(),
            "install_progress": reflash.get_install_progress()
        }
        return data

@app.route('/favicon.ico')
def favicon():
    return flask.send_from_directory(os.path.join(app.root_path, 'static'),
                               'favicon.ico', mimetype='image/vnd.microsoft.icon')
@app.route("/")
def main():
    return flask.render_template('reflash.jinja2')
