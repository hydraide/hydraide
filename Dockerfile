FROM alpine:latest

# Install tools
RUN apk --no-cache add ca-certificates curl shadow su-exec

# Create app folder
WORKDIR /hydraide

# Copy prebuilt binary — this will be injected by the pipeline
COPY hydraide .

# Copy entrypoint script
COPY scripts/entrypoint.sh /entrypoint.sh
RUN chmod +x /entrypoint.sh

ENTRYPOINT ["/entrypoint.sh"]
CMD ["./hydraide"]

HEALTHCHECK --interval=10s --timeout=3s --start-period=3s --retries=3 \
  CMD curl --fail http://localhost:4445/health || exit 1
