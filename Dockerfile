FROM amazonlinux
WORKDIR /root
ADD cache-server /root
ADD config.yml /root
EXPOSE 8080
RUN chmod +x /root/cache-server
CMD /root/cache-server

