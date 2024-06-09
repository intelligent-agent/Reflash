# syntax=docker/dockerfile:1

FROM debian:latest
RUN apt update && apt install -y sudo systemctl debootstrap xz-utils u-boot-tools fdisk cpio
WORKDIR /app
COPY . .
CMD ["./mkimage.sh"]
