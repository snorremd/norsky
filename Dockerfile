FROM scratch
ENTRYPOINT ["/norsky"]
CMD ["serve"]
COPY norsky /