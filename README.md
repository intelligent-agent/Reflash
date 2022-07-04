# Reflash

This is a simple python server that is set up to
get and flash Refactor images

## Setup
### Additional packages for running
apt install \
python3-flask \
python3-requests
pv \
xz \
avahi-daemon \
python3-curses

### Additional packages for installation
apt install \
setuptools \
unzip \



Download source:
wget https://github.com/intelligent-agent/Reflash/archive/refs/heads/main.zip

unzip main.zip
cd Reflash-main
export FLASK_APP="reflash"
flask run --host=0.0.0.0 --port=80

mkdir /opt/images
chown www-data:www-data /opt/images
