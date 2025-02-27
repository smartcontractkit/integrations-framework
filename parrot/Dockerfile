FROM scratch

COPY parrot /app/parrot
ENV PARROT_PORT=80
ENV PARROT_LOG_LEVEL=trace
ENV PARROT_HOST=0.0.0.0
EXPOSE 80

WORKDIR /app

HEALTHCHECK --timeout=3s --interval=30s --start-interval=1ms --start-period=3s CMD [ "./parrot", "health" ]

ENTRYPOINT [ "./parrot" ]
