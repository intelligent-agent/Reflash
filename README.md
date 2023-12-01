# Reflash

This is a simple python server that is set up to
get and flash Refactor images

Latest version is v0.1.1

## Setup
### Additional packages for running
sudo apt install \
python3-flask \
python3-flask-cors \
python3-requests \
pv \
xz \
avahi-daemon \
python3-curses

### Additional packages for installation
apt install setuptools unzip

Download source, then run

```
mkdir -p /opt/reflash/images
mkdir -p /opt/reflash/settings
chown -R www-data:www-data /opt/reflash

make dev-server &
make dev-client
```
