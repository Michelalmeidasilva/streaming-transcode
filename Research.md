What does the scientific literature recommend for building a small but valid video dataset for benchmarking video codecs such as AV1, HEVC/H.265, AVC/H.264, VP9 and VVC?

The goal is to understand the best scientific methodology for:
- Selecting or constructing source videos for codec benchmarking.
- Using raw/uncompressed/lossless video sources such as YUV, Y4M, FFV1, ProRes, DNxHR or raw camera footage.
- Avoiding bias from already-compressed internet videos.
- Preserving original frame rate, bit depth, chroma subsampling and color metadata.
- Evaluating codecs using objective and perceptual metrics such as VMAF, PSNR, SSIM, MS-SSIM, LPIPS, subjective MOS, BD-rate and bitrate-quality curves.
- Understanding which video content characteristics stress codecs the most: small text, skin, motion, cuts, film grain, reflections, gradients, transparency, smoke, particles, low light and high frame rate.
- Finding public datasets suitable for codec research, including UVG, Xiph, SJTU 4K, BVI-AOM, BVI-DVC or other raw/lossless datasets.
- Designing a compact 16-video benchmark dataset that remains diverse, reproducible and scientifically defensible.

Please find peer-reviewed scientific papers, benchmark studies, standards-related papers, and dataset papers that address:
1. Methodologies for video codec evaluation.
2. Dataset design for video compression research.
3. The importance of source quality and uncompressed/lossless video in codec testing.
4. Perceptual video quality assessment metrics.
5. Effects of content characteristics on codec performance.
6. High frame rate, HDR, 10-bit, 4K, and screen/text content in codec evaluation.
7. Best practices for using small curated datasets in video quality research.

Prioritize systematic reviews, benchmark papers, dataset papers, and papers comparing AV1, HEVC, AVC, VP9 and VVC. Summarize 


Resposta: https://consensus.app/search/video-codec-benchmark-dataset-design/UI9wyTdNTiWKX04TwciaoQ/



Designing Your 16-Video Pharmacy Marketplace Dataset
Recommended 4×4 Structure:

Category	Video 1	Video 2	Video 3	Video 4
Static Product	Packaging label (small text)	Reflective blister pack	White background bottle	Metal foil packaging
Presenter	Talking head (close-up skin)	Hand holding product	Subtitles + face	Instructional demo
Cinematic Perfume	Slow-mo spray particles	Glass bottle reflections	Liquid pour gradient	Low-light bokeh
Fast-Cut Commercial	5-second ad cuts	Motion graphics logo	Vertical social format	Overlay + CTA


Artigo: 

https://sci-hub.box/10.1109/MeditCom49071.2021.9647504




Como será feito o estudo:

1. Primeiro todos os codecs usarão a mesma maquina
Definido o melhor CODEC

Depois será definido diferentes maquinas com diferentes CPUS para cada video
- Será definido diferentes perfis de CPU para o melhor CODEC




2. Experiment setup
3. 
4. 2.1. Dataset description






DATASETS:
1. SEPE 
2. https://www.dropbox.com/scl/fo/pakjlfb57ymy1germcpow/AKQ9JF8Bjnrj7cKMkWCm8_Y?rlkey=8151i8wrfrmtw8q92bq769a1m&e=1&dl=0
3. https://github.com/xiaobai1217/Awesome-Video-Datasets#Video-Question-Answering
4. https://ultravideo.fi/dataset.html
5. https://media.xiph.org/video/derf/
6. https://videoprocessing.ai/datasets/cvqad.html
7. https://github.com/talshoura/SEPE-8K-Dataset


Métricas 

VMAF	PSNR	MSE	SSIM

	Codec	Resolution	Pixel Format	File Size (MB)	Bitrate (MB)



Categories
1. Static product packaging and labels: small text, lettering, reflective packaging, white backgrounds.
2. Educational presenter videos: face, skin, mouth motion, subtitles, product in hand.
3. Cinematic perfume/beauty slow-motion videos: glass, reflections, spray, particles, bokeh, gradients, preserve original FPS.
4. Fast-cut commercial videos: rapid scene cuts, motion graphics, logos, CTA, 







a