#/bin/bash
set -e

mkdir -p /opt/reflash/settings
mkdir -p /opt/reflash/images
mkdir -p /opt/reflash/curses
chown -R www-data:www-data /opt/reflash

mkdir -p /var/www/html
cp -r server /var/www/html/
cp -r reflash /usr/local/lib/python3.9/dist-packages/
cp reflash.version /etc
cp systemd/reflash.service /etc/systemd/system
cp systemd/reflash-curses.service /etc/systemd/system
cp curses/client.py /usr/local/bin/reflash-curses.py
chmod +x /usr/local/bin/reflash-curses.py

touch /var/log/reflash.log

FILES="./bin/*"
for f in $FILES
do
  cp $f /usr/local/bin
  f="$(basename -- $f)"
  echo $f
  chmod +x "/usr/local/bin/$f"
done

cat << EOF > /etc/nginx/sites-available/reflash
server {
    listen 80;
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
}
EOF

ln -sf /etc/nginx/sites-available/reflash /etc/nginx/sites-enabled/
rm -f /etc/nginx/sites-enabled/default

systemctl enable reflash
systemctl enable reflash-curses
