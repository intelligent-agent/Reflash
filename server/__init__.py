#!/usr/bin/python3

import os
import flask
import reflash as ref


app = flask.Flask(__name__,
            static_folder = "./dist/static",
            template_folder = "./dist")

if app.config['ENV'] == "development":
    settings = {
        "version_file": ".tmp/etc/reflash.version",
        "images_folder": ".tmp/opt/reflash/images",
        "db_file": ".tmp/opt/reflash/reflash.db",
        "use_sudo": False,
        "platform": "dev",
    }
    with open(settings["version_file"], 'w+') as f:
        f.write("v0.2.0\n")

    from flask_cors import CORS
    CORS(app)
else:
    settings = {
        "version_file": "/etc/reflash.version",
        "images_folder": "/opt/reflash/images",
        "db_file": "/opt/reflash/reflash.db",
        "use_sudo": True,
        "platform": "prod",
    }

with app.app_context():
    reflash = ref.Reflash(settings)
    reflash.start_log_worker()

@app.route('/api/upload_start',methods = ['PUT'])
def upload_start():
    filename = flask.request.json.get("filename")
    size = flask.request.json.get("size")
    start_time = flask.request.json.get("start_time")
    stat = reflash.upload_start(filename, size, start_time)
    return { "success": stat}

@app.route('/api/upload_finish',methods = ['PUT'])
def upload_finish():
    stat = reflash.upload_finish()
    return { "success": stat}

@app.route('/api/upload_cancel',methods = ['PUT'])
def upload_cancel():
    stat = reflash.upload_cancel()
    return { "success": stat}

@app.route('/api/get_upload_progress')
def get_upload_progress():
    return reflash.get_upload_progress()

@app.route('/api/upload_chunk',methods = ['POST'])
def upload_chunk():
    chunk = flask.request.json.get("chunk")
    b64data = chunk.split(",")[1]
    from base64 import b64decode
    decoded_chunk = b64decode(b64data)
    stat = reflash.upload_chunk(decoded_chunk)
    return { "success": stat}

@app.route('/api/check_file_integrity', methods = ['PUT'])
def check_file_integrity():
    filename = flask.request.json.get("filename")
    return reflash.check_file_integrity(filename)

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
    start_time = flask.request.json.get("start_time")
    stat = reflash.backup_refactor(filename, start_time)
    return {"success": stat}

@app.route('/api/cancel_installation', methods = ['PUT'])
def cancel_installation():
    stat = reflash.cancel_installation()
    return {"success": stat}

@app.route('/api/install_refactor', methods = ['PUT'])
def install_refactor():
    filename = flask.request.json.get("filename")
    start_time = flask.request.json.get("start_time")
    stat = reflash.install_refactor(filename, start_time)
    return {"success": stat}

@app.route('/api/get_install_progress')
def get_install_progress():
    return reflash.get_install_progress()

@app.route('/api/download_refactor', methods = ['PUT'])
def download_refactor():
    file = flask.request.json.get("filename")
    start_time = flask.request.json.get("start_time")
    url = file["url"]
    size = file["size"]
    filename = file["name"]
    stat = reflash.download_refactor(url, size, filename, start_time)
    return {"success": stat}

@app.route('/api/cancel_download', methods = ['PUT'])
def cancel_download():
    stat = reflash.cancel_download()
    return {"success": stat}

@app.route('/api/get_info')
def get_info():
    return {
        "local_images": reflash.get_local_releases(),
        "reflash_version": reflash.get_version(),
        "emmc_version": reflash.get_emmc_version(),
        "recore_revision": reflash.get_recore_revision(),
        "usb_present": reflash.is_usb_present(),
        "is_ssh_enabled": reflash.is_ssh_enabled(),
    }

@app.route('/api/get_download_progress')
def get_download_progress():
    return reflash.get_download_progress()

@app.route('/api/reboot_board', methods = ['PUT'])
def reboot_board():
    return reflash.reboot()

@app.route('/api/shutdown_board', methods = ['PUT'])
def shutdown_board():
    return reflash.shutdown()

@app.route('/api/set_ssh_enabled', methods = ['PUT'])
def set_ssh_enabled():
    is_enabled = flask.request.json.get("is_enabled")
    return reflash.set_ssh_enabled(is_enabled)

@app.route('/api/rotate_screen', methods = ['PUT'])
def rotate_screen():
    rotation = flask.request.json.get("rotation")
    where = flask.request.json.get("where")
    restart_app = flask.request.json.get("restart_app")    
    return reflash.rotate_screen(rotation, where, "TRUE" if restart_app else "FALSE")

@app.route('/api/set_boot_media', methods = ['PUT'])
def set_boot_media():
    media = flask.request.json.get("media")
    return reflash.set_boot_media(media)

@app.route('/api/get_boot_media', methods = ['GET'])
def get_boot_media():
    ret = reflash.get_boot_media()
    return {"boot_media": ret}

@app.route('/api/options')
def get_options():
    return reflash.get_options()

@app.route('/api/save_options',methods = ['POST'])
def save_options():
    options = flask.request.json
    return reflash.save_options(options)

@app.route('/api/stream_log',methods = ['GET'])
def stream_log():

    def stream():
        lines = reflash.get_log().split('\n')
        for line in lines:
            yield f"data: {line}\n\n"
        log = reflash.log_listen()
        while True:
            yield log.get()

    return flask.Response(stream(), mimetype='text/event-stream')

@app.route('/api/clear_log', methods=['PUT'])
def clear_log():
    return reflash.clear_log()

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


