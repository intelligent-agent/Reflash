#/bin/bash
set -e

apt install -y nginx gunicorn python3-flask

mkdir -p /opt/reflash/settings
mkdir -p /opt/reflash/images
chown -R www-data:www-data /opt/reflash

mkdir -p /var/www/html
cp -r reflash /var/www/html/
cp systemd/reflash.service /etc/systemd/system
FILES="./bin/*"
for f in $FILES
do
  cp $f /usr/local/bin
  f="$(basename -- $f)"
  echo $f
  chmod +x "/usr/local/bin/$f"
done

chown -R www-data:www-data /var/www/html/reflash

cat << EOF > /etc/nginx/sites-available/reflash
server {
    listen 80;
    server_name _;
    client_max_body_size 2M;

    location / {
        proxy_pass http://127.0.0.1:8000/;
        proxy_set_header X-Forwarded-For $$proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $$scheme;
        proxy_set_header X-Forwarded-Host $$host;
        proxy_set_header X-Forwarded-Prefix /;
    }
}
EOF

sudo ln -sf /etc/nginx/sites-available/reflash /etc/nginx/sites-enabled/
sudo rm -f /etc/nginx/sites-enabled/default

systemctl enable reflash
systemctl restart reflash
systemctl restart nginx
