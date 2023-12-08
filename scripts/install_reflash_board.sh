#/bin/bash
set -e -x

mkdir -p /opt/reflash/settings
mkdir -p /opt/reflash/images
mkdir -p /opt/reflash/curses
chown -R www-data:www-data /opt/reflash

mkdir -p /var/www/html
cp -r server /var/www/html/
cp -r reflash /usr/local/lib/python3.9/dist-packages/
cp reflash.version /etc
cp systemd/reflash.service /etc/systemd/system
cp curses/client.py /usr/local/bin/reflash-curses.py

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
    listen 8080;
    server_name _;
    client_max_body_size 10M;

    location / {
        proxy_pass http://127.0.0.1:8000/;
        proxy_set_header X-Forwarded-For $$proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $$scheme;
        proxy_set_header X-Forwarded-Host $$host;
        proxy_set_header X-Forwarded-Prefix /;
    }
}
EOF

ln -sf /etc/nginx/sites-available/reflash /etc/nginx/sites-enabled/
rm -f /etc/nginx/sites-enabled/default

systemctl enable reflash
