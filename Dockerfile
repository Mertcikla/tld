FROM gcr.io/distroless/static-debian13:nonroot
COPY --chmod=755 tld /tld
ENV TLD_HOST=0.0.0.0 PORT=8060 \
    TLD_DATA_DIR=/home/nonroot/data TLD_CONFIG_DIR=/home/nonroot/config \
    TLD_UPDATES_AUTO=false TLD_SKIP_STARTUP_UPDATE=1
WORKDIR /home/nonroot
EXPOSE 8060
ENTRYPOINT ["/tld"]
CMD ["serve", "--foreground"]
