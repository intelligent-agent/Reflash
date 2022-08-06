install_bins:
	cp bin/dev/* /usr/local/bin
	chmod +x /usr/local/bin/*

dev:
	FLASK_ENV="development" \
	FLASK_APP="reflash" \
	flask run --host=0.0.0.0 --port=8081

client-dev:
	cd client; npm run serve

build:
	cd client; npm run build

upload:
	scp -r client/dist root@recore.local:/var/www/html/reflash
	scp reflash/*.py root@recore.local:/var/www/html/reflash
	scp systemd/*.service root@recore.local:/etc/systemd/system/

tar: gzip
	cd zip; tar -zcvf reflash.tar.gz reflash/
	mv zip/reflash.tar.gz .
	rm -rf zip

gzip:
	mkdir -p zip/reflash/bin
	cp -r client/dist zip/reflash
	cp reflash/*.py zip/reflash
	cp bin/prod/* zip/reflash/bin
	cp -r systemd zip/reflash

upload-tar:
	scp reflash.tar.gz root@recore.local:/var/www/html/
