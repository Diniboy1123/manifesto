# manifesto

Manifesto is a real-time streaming format translator that converts [Microsoft Smooth Streaming](https://learn.microsoft.com/en-us/openspecs/windows_protocols/ms-sstr/8383f27f-7efe-4c60-832a-387274457251) (`.ism`) manifests into widely supported [MPEG-DASH](https://www.iso.org/standard/83314.html) (`.mpd`) manifests.

It also handles init segment generation and on-the-fly segment repackaging, all written in pure Go—making it lightweight and able to run on resource-constrained devices like the [Raspberry Pi Zero](https://www.raspberrypi.com/products/raspberry-pi-zero/).

Manifesto supports repackaging of DRM-protected content without decryption. If decryption keys are provided, it can optionally decrypt as well.

## Table of contents
- [manifesto](#manifesto)
  - [Table of contents](#table-of-contents)
  - [Building from source](#building-from-source)
    - [Docker](#docker)
  - [Usage](#usage)
    - [Configuration](#configuration)
      - [Fields](#fields)
    - [Playback](#playback)
  - [Why? Why was this built?](#why-why-was-this-built)
    - [How?](#how)
  - [Player support](#player-support)
  - [Quick comparison table:](#quick-comparison-table)
  - [In depth comparison:](#in-depth-comparison)
    - [FFmpeg](#ffmpeg)
    - [mpv](#mpv)
    - [MX Player on Android](#mx-player-on-android)
    - [VLC](#vlc)
    - [InputStream Adaptive (Kodi)](#inputstream-adaptive-kodi)
    - [dash.js](#dashjs)
  - [Supported codecs](#supported-codecs)
  - [Modes of operation](#modes-of-operation)
  - [Various hacks applied](#various-hacks-applied)
    - [Hijacked init and segment URLs](#hijacked-init-and-segment-urls)
    - [Track IDs inside segments are always set to 1](#track-ids-inside-segments-are-always-set-to-1)
    - [`tfdt` box is added to segments if missing](#tfdt-box-is-added-to-segments-if-missing)
    - [`DataOffset` in `trun` box is always reset to 0](#dataoffset-in-trun-box-is-always-reset-to-0)
  - [Performance](#performance)
    - [Caching](#caching)
  - [Stand on piracy](#stand-on-piracy)
  - [Contributing](#contributing)
  - [Acknowledgements](#acknowledgements)

## Building from source

Since the tool is written in Go, building it should be straightforward on any system with a recent Go environment.

1. Ensure that you have Go installed on your system. You can download it from [here](https://golang.org/dl/). At least Go 1.23 is required.
2. Clone this repository and switch to the project's root directory
3. Build the project:
```shell
CGO_ENABLED=0 go build -ldflags="-s -w" .
```

And that will produce an `manifesto` binary in the current directory.

If you would rather cross compile, set the `GOOS` and `GOARCH` environment variables accordingly. For example, to build for Windows on a Linux system:
```shell
GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go build -ldflags="-s -w" .
```

### Docker

You can deploy the tool using Docker. [Dockerfile](Dockerfile) is provided in the repository. To build the image, run:

```shell
docker build -t manifesto:latest .
```

Example usage:

```shell
docker run -it --rm -v $HOME/manifesto/config:/config -p 8080:8080 manifesto:latest -config /config/config.json
```

## Usage

This tool is designed as a web service, not a command-line utility. Refer to [Configuration](#configuration) first and once ready, bring up the service with:

```shell
./manifesto -config /path/to/config.json
```

> [!NOTE]
> If no `-config` is provided, the service will look for a file called `config.json` in the current directory.

### Configuration

For simplicity, the tool uses a JSON configuration file. The default file is `config.json` in the current directory. You can specify a different file using the `-config` flag.

Example config:

```json
{
    "http_port": 8080,
    "bind_addr": "0.0.0.0",
    "save_dir": "/tmp/data",
    "log_path": "/tmp/logs.txt",
    "allow_subs": true,
    "cache_duration": "3s",
    "global_headers": {
        "X-Requested-With": "manifesto"
    },
    "https_port": 8443,
    "bogus_domain": "notgonnaexpo.se",
    "hide_not_found": true,
    "http_proxy": "socks5h://some@example.com:1080",
    "https_proxy": "http://another:simple@example.com:8080",
    "tls_domain_map": [
        {
            "domain": "my.domain.tld",
            "cert": "fullchain.pem",
            "key": "privkey.pem"
        }
    ],
    "users": [
        {
            "username": "admin",
            "token": "VERYSECUREHOPEFULLYRANDOMTOKEN"
        }
    ],
    "channels": [
        {
            "id": "magentatest",
            "source_type": "ism",
            "destination_type": "mpd",
            "name": "Magenta Test",
            "url": "https://smstr01.dmm.t-online.de/smooth24/smoothstream_m1/streaming/sony/9221438342941275747/636887760842957027/25_km_h-Trailer-9221571562372022953_deu_20_1300k_HD_H_264_ISMV.ism/Manifest"
        },
        {
            "id": "mstest",
            "source_type": "ism",
            "destination_type": "mpd",
            "name": "Microsoft Encrypted Test",
            "url": "https://test.playready.microsoft.com/media/profficialsite/tearsofsteel_4k.ism.smoothstreaming/manifest",
            "keys": ["6f651ae1dbe44434bcb4690d1564c41c:88da852ae4fa2e1e36aeb2d5c94997b1"]
        }
    ]
}
```

#### Fields

- `http_port`: The port on which the service will listen. If set to `0` or if omitted, HTTP will be disabled.
- `https_port`: The port on which the service will listen for HTTPS connections. If set to `0` or if omitted, HTTPS will be disabled.
- `bind_addr`: Bind address to expose the service to. If set to `127.0.0.1` only local connections will be accepted, if set to `0.0.0.0` all connections are accepted. If set to a specific interface's IP address, only connections coming from that interface will be accepted.
- `save_dir`: Directory where cached requests will be saved for the `cache_duration` time. The tool deletes all files stored here during each startup, so don't put anything important here. The directory will be created if it doesn't exist.
- `log_path`: Path to the log file. If not set, only stdout will be used.
- `allow_subs`: If set to `true`, subtitles will be included in transformed manifest files. Some players like ffmpeg, may behave unpredictably when STPP subtitles are present in the manifest, therefore all subtitles are stripped by default.
- `cache_duration`: Duration for which the cached requests will be saved. All calls done by the service will be cached for this duration including source manifests. Choose a value wisely. Too less and you end up hammering the source. Too much and you end up with a lot of data on your disk + manifests will serve stale data.
- `global_headers`: HTTP headers that will be added to all requests. This is useful for authentication or other purposes. The headers are passed as a map of key-value pairs. Not necessary if you don't need any headers.
- `http_proxy`: Proxy to use for outgoing HTTP traffic. This is useful if you want to route all requests through a proxy. The proxy is passed as a string in the format `protocol://username:password@host:port`. If not set, no proxy will be used. Equivalent to `HTTP_PROXY` environment variable.
- `https_proxy`: Proxy to use for outgoing HTTPS traffic. This is useful if you want to route all requests through a proxy. The proxy is passed as a string in the format `protocol://username:password@host:port`. If not set, no proxy will be used. Equivalent to `HTTPS_PROXY` environment variable.
- `tls_domain_map`: List of domains and their corresponding TLS certificates. This is useful if you want to serve multiple domains with different certificates.
  - `domain`: Domain name to serve the certificate for. If the request's SNI matches this domain, the certificate will be used.
  - `cert`: Path to the certificate file for a specific domain. The file will be read and used for TLS connections.
  - `key`: Path to the private key file for a specific domain. The file will be read and used for TLS connections.
- `bogus_domain`: The service generates a self-signed certificate which will be served on the HTTPS port if no known SNI is given. This ensures that random port scanners won't find out the domain you are hosting on. If not set, the certificate will not contain any subject alternative names.
- `hide_not_found`: If set to `true`, the service will return 204 No content to all unknown pathes. If set to `false`, regular 404 Not Found will be returned. Also useful against port scanners.
- `users`: List of users that can access the service. Each user has a `username` and a `token`. The token is used for authentication. If defined, the service will require a token in each call in the path e.g. `/mysecuretoken/stream/...`. If not defined, the service will be open to everyone. Username is only used for logging purposes.
- `channels`: List of channels that the service will serve.
  - `id`: Unique ID of the channel. This is used in the URL to access the channel.
  - `source_type`: Type of the channel. Currently only `ism` is supported and the field is unused. Please set it regardless in case the tool is extended to support other formats in the future.
  - `destination_type`: Type of the destination manifest. Currently only `mpd` is supported and the field is unused. Please set it regardless in case the tool is extended to support other formats in the future.
  - `name`: Pretty name for the channel. Currently unused, but will be used in the future to display names and render channel lists.
  - `url`: URL of the source manifest. This is the URL that will be transformed to DASH.
  - `keys`: List of keys in hex format that will be used to decrypt the content. The keys are passed as a list of strings. Each key is a string in the format `key_id:key`. The key_id is the ID of the key and the key is the actual key. For now only one key is supported. If left unspecified, the service will look into manifests and if it notices that the manifest is encrypted, it will not attempt to strip encryption. If it sees an unencrypted manifest, it will serve the unencrypted data.

### Playback

If you added the `magentatest` example and the service is running on `localhost:8080`, you can run for example [VLC](https://www.videolan.org/vlc/) simply:

```shell
vlc http://localhost:8080/stream/magentatest/manifest.mpd
```

And the the playback should start.

## Why? Why was this built?

I like to follow local sports events and my local TV station uses Smooth Streaming to deliver the content. As long as I had an LG TV, I had no issues accessing the content (as they have an official app there), but when I switched to an Android TV I was left with no option to watch the content I pay for.

My new Android TV supports PlayReady DRM, which is exactly what the content is protected with, but there is no smooth streaming support there and while Kodi's [inputstream.adaptive](https://github.com/xbmc/inputstream.adaptive) has some degree of support for Smooth Streaming, it is not perfect and won't work with the provider's manifests.

I spent a long while researching this topic and found countless tools that are able to download and decrypt Smooth Streaming content, but none of them were able to just convert the manifest to DASH and serve it. I have no keys nor interest in decrypting the content, I just wanted to be able to watch the content. The only tool I found for this purpose was [DashMe](https://github.com/canalplus/DashMe), which is a great inspiration, but doesn't even compile anymore and relies on [FFmpeg](https://www.ffmpeg.org/) plus does some degree of remuxing, therefore making it quite resource hungry.

This tool was built to fill the gap.

### How?

It does three things. Let's say you have a `magentatest` channel defined in the config and you have the service bound to port `8080`. This will result in a URL like this:

```
http://localhost:8080/stream/magentatest/manifest.mpd
```

This brings us to the first step. Any request made to these manifest endpoints will result in a request to the upstream provider to fetch the MSS manifest, which is then cached, parsed and transformed to a DASH manifest. The code tries to port all important fields and supports multiple resolutions, audio tracks and subtitles. While most properties are kept, init segments and chunk URLs are hijacked to point to the local machine, so it can serve those requests in the future as well. The manifest is then served to the client.

As the second step a regular player that plays MPEG-DASH would reach out to is the URL of the init segment. However MSS doesn't have a concept of a pre-served init segment, but it is rather generated on the client side. DASH however needs the init segment, so the tool attempts to generate the init segment on the fly. This is done by parsing various properties from the manifest, most importantly the `CodecPrivateData` field, which usually contains the codec specific information. These init segments are also served back to the client on a per-request basis. Since init segment generation needs to be programmed for each codec, only a few codecs are supported. Currently the tool supports `H264` (`avc1`) for video, `AAC` and `EAC-3` for audio and `STPP` for subtitles. For example I didn't encounter HEVC streams yet, so those are not implemented. If you encounter a codec that is not supported, please open an issue and I will try to implement it.

The third step is the actual segment request. If a player requests a segment, the tool will reach out to the upstream provider and fetch the given segment. This can't be served as-is, because some MP4 boxes need to be altered and removed, so the segments are also parsed, repackaged and served on the fly. For example we replace track IDs to always be `1`, because the generated init segments also always have track ID `1`. Certain players have audio/video desync issues if you don't specify a `tfdt` box, so we add that if missing as well.

With all that, `manifesto` is able to serve a DASH manifest that should be playable without any re-encoding or remuxing.

## Player support

Experience shows that unfortunately not all players are able to play the generated DASH manifest. Here is a list of my experience so far. Keep in mind that this experience was with non DRM protected content. The only player that I tested with DRM protected content was InputStream Adaptive inside Kodi.

Quick comparison table:
--

| Player                      | Status          | Notes                                                                                           |
| --------------------------- | --------------- | ----------------------------------------------------------------------------------------------- |
| FFmpeg                      | Unusable        | Subtitle requests go into an infinite loop. Playback is stuttery and desync issues are present. |
| mpv                         | Unusable        | Same issue as FFmpeg. Playback won't start before fetching a large amount of segments.          |
| MX Player on Android        | Unusable        | Playback starts, but runs into issues and eventually gives up.                                  |
| VLC                         | Works perfectly | Segments are modified by the tool. Playback is smooth and without issues.                       |
| InputStream Adaptive (Kodi) | Works perfectly | Playback is smooth and without issues.                                                          |
| dash.js                     | Works perfectly | Playback is smooth and without issues. Segments are modified by the tool.                       |

In depth comparison:
--

### FFmpeg

**Unusable**. Both `ffprobe` and `ffplay` go into a seemingly infinite cycle of requests if you have `allow_subs` set to `true`. For some reason, subtitle chunks are requested in an infinite loop. If you set `allow_subs` to `false`, it gets as far as to start playing something, first it starts smooth, but then audio and video go out of sync, you will experience random jumps and stutters. I assume FFmpeg is just not smart enough to rely on `tfdt` boxes or the times inside the manifest, so timestamps are extracted from the actual `mdat` boxes inside the segments which is obviously wrong. After playing livestreams for a longer while, you may also observe that FFmpeg tries to fetch chunks that aren't even available yet on the upstream, so desync issues are definitely present. Unfortunately most players that depend on FFmpeg suffer from the same issue.

### mpv

**Unusable**. Same issue as with FFmpeg, sometimes even worse as it tries to pre-load the content and fails to play anything.

### MX Player on Android

**Unusable**. The player acts very similarly to FFmpeg. Playback usually starts, but it runs into issues and eventually gives up on my end. Then screen goes blank even though it tries to fetch segments still.

### VLC

**Works perfectly, but segments are modified by the tool for that**. VLC had similar issues like FFmpeg at first, though playback was mostly seamless. Audio was noticably late though and sometimes either the video or the audio track was cut off presumably to sync up. This is because VLC seemingly also ignores timings set in manifests and solely relies on the individual segments for timestamps. After lots of research, I figured that if I add a `tfdt` box to the segments with the time fetched from the manifests (which is kind of hacky, but where else would I get timestamps from), VLC is able to play the content without any issues. If there is already a `tfdt` box in the segment, it is left as-is.

Playback has been tested on both Linux and Android versions and it was perfect. Even subtitles and multiple tracks work.

### InputStream Adaptive (Kodi)

**Works perfectly.** It was the first and most important goal to have it working there. Surprisingly the playback pipeline is the most mature there for live manifests from all the players I have tested so far and it worked on the first try without any special quirks or issues. It is also the only open-source player that is able to play DRM protected content.

Please note that there are huge differences between Kodi/ISA versions here so always be sure to have the latest version to ensure a smooth playback experience.

### dash.js

Tested [here](https://reference.dashif.org/dash.js/nightly/samples/dash-if-reference-player/index.html).

**Works perfectly with the same quirk as VLC.** The player is able to play the content without any issues, but it also relies on the `tfdt` box to be present in the segments. If you remove it, playback won't even start. At least based on my experience. Once it's there, playback is smooth and without any issues.

The only quirk I have encountered is this warning on livestreams:

```
Warning : No valid segment found when applying a specification compliant DVR window calculation. Using SegmentTimeline entries as a fallback.
```

## Supported codecs

So far these are known to work (and not):

| Type     | Codec | Supported |
| -------- | ----- | --------- |
| Video    | avc1  | Yes       |
| Audio    | aac   | Yes       |
| Audio    | eac3  | Yes       |
| Subtitle | stpp  | Yes       |
| Video    | hevc  | No        |

If you encounter a codec that is not supported, please open an issue and I will try to implement it. I just haven't encountered such a manifest yet.

## Modes of operation

The tool is special, because it is able to serve PlayReady protected manifests and chunk data without decryption. So clients like Inputstream Adaptive can be initialized on supported Android TVs and you can watch content legally on unsupported devices even. If you specify an originally PlayReady protected manifest, the tool will recognize that and generate init segments with the encryption data present. Segments also won't be decrypted, but served as-is with minor modifications only. This can be achieved if you don't specify any keys for the specific channel in the config. The manifest should also contain PR specific custom data, otherwise it will be treated as unencrypted media.

If you specify keys for a specific channel and the extracted keyId from PR customData matches the keyId you provided, the tool will attempt to decrypt the content using the specified key. Normally you don't have raw keys as obtaining those is usually illegal or not possible even. I don't posess keys for my provider's content either, but this mode was very useful for testing. I provided random dummy keys that end up decrypting random data and figured that my provider sets the `mdat` data offset at `Moof.Traf.Trun.DataOffset` to a wrong value which makes mp4ff, the underlying library used by this tool for MP4 box parsing and encoding panic and also causes Kodi's ISA based MSS player to fail.

Lastly, you can also specify a channel with no keys and a manifest that is not DRM protected. In this case, the tool will serve the unencrypted content as-is, just like it would with a DRM protected manifest.

For DRM protected content, only PlayReady is supported. I haven't encountered Widevine protected stuff yet, so I couldn't implement that. It's not really a priority for me either.

## Various hacks applied

### Hijacked init and segment URLs

All init and segment pathes are hijacked in all manifests. I modify those pathes to contain exactly enough data for the server to know which track to request, which qualitylevel we are requesting and in the segment fetching case, which timestamp are we currently requesting. Therefore it's very important that the origin has a `$RepresentationID$`, stream indexes in the original MSS manifest don't change order and timestamps are correctly calculated.

It's awful, it's an abuse of the MSS protocol, but this way I can serve multiple streams statelessly.

### Track IDs inside segments are always set to 1

When generating the init segment, there is hardly any information available for us to know, which track ID we are generating an init segment for. But then segments will contain whatever track ID the origin had. Most players crash here, they see an init segment with track ID `1` and then they see a segment with track ID `2` and they don't know what to do. So I just modify each and every segment to always have track ID `1`. This way the player will always see the same track ID and it won't crash. I am surprised too, but this works just fine.

### `tfdt` box is added to segments if missing

My provider serves video and audio tracks with separate timestamps. Some players, like Inputstream Adaptive inside Kodi, are able to handle that, and can solely rely on whatever timestamps each segment has inside the manifests. But some players, like VLC and dash.js, are not able to handle that and they need a `tfdt` box inside the segments to know when the segment starts. For that, I would need to know the timestamp of the currently requested segment though. So I do the awful hack of injecting the timestamp extracted from the manifest into the request URL and then I add a `tfdt` box to the segment with this timestamp. This way the player is happy and playback is smooth. If the `tfdt` box is already present, it is left as-is.

### `DataOffset` in `trun` box is always reset to 0

I have seen some providers setting the `DataOffset` in the `trun` box of certain audio tracks to a wrong value. It ended up panicing the mp4ff library and also caused Kodi's ISA based MSS player to fail (without using `manifesto` even, just directly passing the origin manifests). I read the specs and ended up with the conclusion that this isn't even allowed.

So I just reset the `DataOffset` to `0` in all `trun` boxes and let mp4ff calculate the correct value.

All tested segments played this way just fine, so I ended up making this a default behavior. If you encounter a segment that doesn't play this way, please open an issue and I will move this to a config option.

## Performance

The tool is pure Go and doesn't remux anything, therefore it is very lightweight and fast compared to other tools. However it still loads large chunks of data into memory, so it may not be suitable for low-end devices. On the contrary, I am running this on a Raspberry Pi Zero W and it works just fine. Since I would like to keep it that way, I do not have plans to implement FFmpeg based timestamp calculation. It would be nice to have, as that would open up the possibility to support more players, but less resource hungry and faster is more important to me.

### Caching

The tool caches all requests for the duration of `cache_duration` in the config to the `save_dir` directory. This is inevitable with the current design as I wanted to keep things stateless. Both init generation and segment repackaging rely on information extracted from the manifest, so I need to keep the manifest around. And the easiest solution to that was to simply cache every request done by the service. This brings in the nice side effect of being able to serve the same manifest/segment data multiple times without having to reach out to the origin again.

But it's also harmful to disk wear. If you run this on a Raspberry with some SD card, I suggest using `tmpfs` for the `save_dir` directory so stuff goes into RAM and doesn't wear out your SD card. If you have lots of concurrent streams or large chunks, that may won't suffice, but then you should look at upgrading your setup.

## Stand on piracy

**I do not condone piracy in any way.** Always make sure you have the right to access the content you are trying to play. This tool doesn't provide tools to circumvent DRM. All it does is translating a manifest from one format to another. You still need a device that supports PlayReady DRM to be able to play PR protected content.

I will not provide support for any illegal activities. If you are trying to use this tool to obtain content you are not entitled to, please do not open issues or ask for help. I will not help you.

## Contributing

Contributions are welcome. In fact I am a university student with very limited time and resources. For now the tool mostly implements my needs and ideas, but I would like to see it grow and become more stable with many exciting features to come. If you have any ideas, suggestions, bug reports or even code contributions, feel free to open an issue or a pull request. I will do my best to get back to you.

## Acknowledgements

This tool wouldn't exist without the following incredible projects. Please go and star them all if you like this project!

- [Bento4](https://github.com/axiomatic-systems/Bento4) - The `mp4dump` tool was used really often during development to check the generated segments. I also found out, how to parse EAC-3 `CodecPrivateData` thanks to [this](https://github.com/axiomatic-systems/Bento4/blob/3bdc891602d19789b8e8626e4a3e613a937b4d35/Source/Python/utils/mp4utils.py#L1047-L1054) snippet. There is really nowehere else on the internet where I could find this information.
- [DashMe](https://github.com/canalplus/DashMe) - Similar project to this one, but doesn't compile anymore and relies on FFmpeg. It was a great inspiration for this project however.
- [Go](https://go.dev/) - The Go programming language. I am not a professional developer, but I was able to get this working with Go. It is a great language.
- [go-mpd](https://github.com/unki2aut/go-mpd) - I took code and inspiration for the MPD serialization. Really useful lib.
- [go-xsd-types](github.com/unki2aut/go-xsd-types) - Useful small project I use mainly to parse DASH specific durations to Go time.Duration types.
- [fsnotify](github.com/fsnotify/fsnotify) - Used to watch the config file for changes in a cross platform way. Super handy!
- [MagNumDB](https://www.magnumdb.com) - Helped me figure out the UUID for EAC-3 audio codec.
- [Microsoft Docs](https://learn.microsoft.com/en-us/openspecs/windows_protocols/ms-sstr/8383f27f-7efe-4c60-832a-387274457251) - The official Smooth Streaming protocol documentation. I used it to figure out how the protocol works and how to parse the manifests. It is surprisingly detailed. [This](https://learn.microsoft.com/en-us/previous-versions/dd318793(v=vs.85)) was also useful to learn, how to parse the `CodecPrivateData` field for AAC audio.
- [mp4ff](https://github.com/Eyevinn/mp4ff) - Undoubtedly my favorite Go library so far. It's written excellently and has a great API to work with. I use it to parse and generate MP4 boxes. It is also a great source of information as I don't have the money to buy the specs for the MP4 format. Without this project, this tool definitely wouldn't exist. It makes MP4 parsing and generation a breeze. Directly from Go.
- [yt-dlp](https://github.com/yt-dlp/yt-dlp/blob/74e90dd9b8f9c1a5c48a2515126654f4d398d687/yt_dlp/downloader/ism.py#L159) - The `ism.py` file was used to figure out how to parse `avc1` `CodecPrivateData` and extract SPSNALUs and PPSNALUs. It's also a great example on init segment generation, although mp4ff does a much better job at covering the standards and implementing them.