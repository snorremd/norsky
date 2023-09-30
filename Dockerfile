FROM scratch

ENTRYPOINT ["/norsky"]
CMD ["serve"]

VOLUME [ "/norsky" ]

EXPOSE 3000
COPY norsky /