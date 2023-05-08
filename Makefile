install_bins:
	cp bin/dev/* /usr/local/bin
	chmod +x /usr/local/bin/backup-emmc
	chmod +x /usr/local/bin/enable-emmc-ssh
	chmod +x /usr/local/bin/flash-recore
	chmod +x /usr/local/bin/reboot-board
	chmod +x /usr/local/bin/shutdown-board
	chmod +x /usr/local/bin/set-boot-media

dev-server:
	FLASK_RUN_PORT=8081 \
	FLASK_ENV=development \
	FLASK_DEBUG=1 \
	flask --app server run

dev-clean:
	rm -rf .tmp
	mkdir -p .tmp/opt/reflash/images
	mkdir -p .tmp/dev/
	mkdir -p .tmp/etc/
	dd if=/dev/random of=.tmp/dev/mmcblk0 count=1000 bs=1M
	echo "0.1.2" > .tmp/etc/reflash_version

dev-client:
	cd client; npm run serve

build:
	cd client; npm run build

upload:
	scp -r client/dist root@recore.local:/var/www/html/reflash
	scp reflash/*.py root@recore.local:/var/www/html/reflash
	scp systemd/*.service root@recore.local:/etc/systemd/system/

tar: package
	cd zip; tar -zcvf reflash.tar.gz reflash/
	mv zip/reflash.tar.gz .
	rm -rf zip

package:
	mkdir -p zip/reflash/bin
	mkdir -p zip/reflash/reflash
	cp reflash/*.py zip/reflash/reflash
	mkdir -p zip/reflash/server
	cp -r server/*.py zip/reflash/server
	cp -r client/dist zip/reflash/server
	cp bin/prod/* zip/reflash/bin
	cp -r systemd zip/reflash
	cp -r scripts zip/reflash
	cp -r u-boot zip/reflash
	cp -r curses zip/reflash

upload-tar:
	scp reflash.tar.gz root@recore.local:/usr/src/

tests:
	python3 -m pytest tests

.PHONY: tests