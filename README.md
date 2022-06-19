# Reflash

This is a simple python server that is set up to
get and flash Refactor images

## Setup

apt install \
setuptools \
unzip \
python3-flask \
python3-requests

Download source:
wget https://github.com/intelligent-agent/Reflash/archive/refs/heads/main.zip

unzip main.zip
cd Reflash-main
export FLASK_APP="reflash"
flask run --host=0.0.0.0 --port=80

mkdir /opt/images
chown www-data:www-data /opt/images
