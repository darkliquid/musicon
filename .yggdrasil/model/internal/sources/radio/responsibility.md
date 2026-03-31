# Responsibility

This node owns Musicon's Radio Browser-backed internet radio source.

It is responsible for:

- exposing Radio Browser-backed station search results to queue management
- translating Radio Browser station metadata into Musicon search, queue, playback, and cover-art metadata
- resolving queued radio entries into live playable streams
- selecting the playback path for each station, including direct in-process decode for simple streams and native Go handling for HLS or ADTS AAC streams Musicon cannot decode through the direct decoders
- preserving the user-facing product goal that internet radio support should not stop at only the easiest direct-stream formats

# Boundaries

This node is not responsible for:

- rendering queue or playback UI
- outputting audio to speakers
- implementing reusable cover-art resolution providers
- managing TOML parsing or startup configuration discovery
- owning the audio engine's seek, buffering, or transport-control policies
