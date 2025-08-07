FROM alpine:latest

ARG TARGETARCH

# Install tools
RUN apk --no-cache add ca-certificates curl shadow su-exec

# Create app folder
WORKDIR /hydraide

# Copy the correct binary for the platform
COPY hydraide-${TARGETARCH} ./hydraide

# Copy entrypoint script
COPY scripts/entrypoint.sh /entrypoint.sh
RUN chmod +x /entrypoint.sh
RUN chmod +x /hydraide/hydraide

ENTRYPOINT ["/entrypoint.sh"]
CMD ["./hydraide"]

HEALTHCHECK --interval=10s --timeout=3s --start-period=3s --retries=3 \
  CMD curl --fail http://localhost:4445/health || exit 1
