FROM scratch

ENTRYPOINT ["/norsky"]
CMD ["serve"]
EXPOSE 3000
COPY norsky /