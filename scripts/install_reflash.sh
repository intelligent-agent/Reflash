#/bin/bash
set -e

mkdir -p /opt/reflash/settings
mkdir -p /opt/reflash/images
chown -R www-data:www-data /opt/reflash

mkdir -p /var/www/html/

echo <<EOF > /etc/nginx/sites-available/reflash
server {
    listen 80;
    server_name _;

    location / {
        proxy_pass http://127.0.0.1:8000/;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
        proxy_set_header X-Forwarded-Host $host;
        proxy_set_header X-Forwarded-Prefix /;
    }
}
EOF

sudo ln -s /etc/nginx/sites-available/reflash /etc/nginx/sites-enabled/
sudo rm /etc/nginx/sites-enabled/default

apt install python3-flask python3-gunicorn
