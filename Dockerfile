FROM amazonlinux:2023.6.20250107.0
WORKDIR /root
ADD cache-server /root
ADD config.yml /root
EXPOSE 8080
RUN chmod +x /root/cache-server
CMD /root/cache-server

