Como será feito o estudo:

Convertido em 4 resoluções diferentes:
1280x720
1366x768
1600x900
1920x1080

Testar em duas faixas de bitrates:
Low-bitrate - 1000 kpbs
High-Bitrate - 6000 kbps



Metodologia



Dificuldades:
- métrica de qualidade, precisaria de uma pesquisa humana 


https://arxiv.org/pdf/2211.12109


As categorias de videos estão definidas em: 

Vídeos de comerciais com vários cortes
Vídeos educacionais com pessoas falando e com legendas
Videos mais estáticos com nenhum ou quase nenhum corte




To analyze the relevance of quality metrics and performance for video compression, 

We downloaded only videos that were available
under CC BY and CC0 licenses and that had a minimum bitrate of 20 Mbps
We converted all videos to a YUV 4:2:0 chroma subsampling.


encoders: 
h.264
h.265
av1


Major streaming-video services
recommend at most 4,500–8,000 kbps for FullHD encoding [3, 4, 5].

We compressed each video at three target
bitrates — 1,000 kbps, 2,000 kbps, and 4,000 kbps



3.1.2 Full-Reference Video-Quality Metrics
PSNR and SSIM [45] are among the most popular image- and video-quality metrics. We compared
variations of SSIM and MS-SSIM [46] in our benchmark; the latter is an advanced version of the....



3. VIdeo dataset
To analyze the relevance of quality metrics to video compression, we collected a special dataset of
videos exhibiting various compression artifacts. For video-compression-quality measurement, the
original videos should have a high bitrate or, ideally, be uncompressed to avoid recompression artifacts.


We chose from a pool of more than 18,000 high-bitrate open-source videos from www.vimeo.com.
Our search included a variety of minor keywords to provide maximum coverage of potential results—
for example “a,” “the,” “of,” “in,” “be,” and “to.” We downloaded only videos that were available
under CC BY and CC0 licenses and that had a minimum bitrate of 20 Mbps