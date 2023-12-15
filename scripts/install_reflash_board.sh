#/bin/bash
set -e -x

SCRIPT=$(realpath "$0")
SROOT=$(dirname "$SCRIPT")/..

mkdir -p /opt/reflash/settings
mkdir -p /opt/reflash/images
mkdir -p /opt/reflash/curses
chown -R www-data:www-data /opt/reflash

mkdir -p /var/www/html
cp -r $SROOT/server /var/www/html/
cp -r $SROOT/reflash /usr/local/lib/python3.9/dist-packages/
cp $SROOT/reflash.version /etc
cp $SROOT/systemd/reflash.service /etc/systemd/system

touch /var/log/reflash.log

FILES="$SROOT/bin/*"
for f in $FILES
do
  cp $f /usr/local/bin
  f="$(basename -- $f)"
  echo $f
  chmod +x "/usr/local/bin/$f"
done

cat << EOF > /etc/nginx/sites-available/reflash
server {
    listen 8080;
    server_name _;
    client_max_body_size 10M;

    location / {
        proxy_pass http://127.0.0.1:8000/;
        proxy_set_header X-Forwarded-For \$proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto \$scheme;
        proxy_set_header X-Forwarded-Host \$host;
        proxy_set_header X-Forwarded-Prefix /;
        proxy_http_version 1.1;
        proxy_set_header Connection "";
        chunked_transfer_encoding off;
        proxy_buffering off;
        proxy_read_timeout 24h;
    }

    allow 10.0.0.0/8;
    allow 127.0.0.0/8;
    allow 169.254.0.0/16;
    allow 172.16.0.0/12;
    allow 192.168.0.0/16;
    allow FE80::/10;
    allow ::1/128;
    deny all;
}
EOF

ln -sf /etc/nginx/sites-available/reflash /etc/nginx/sites-enabled/
rm -f /etc/nginx/sites-enabled/default

systemctl enable reflash
