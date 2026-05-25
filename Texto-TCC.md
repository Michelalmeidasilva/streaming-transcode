Proposta metodológica para construção de um dataset realista de vídeos para avaliação de codecs em marketplace farmacêutico

A construção de um dataset para avaliação de codecs de vídeo deve considerar não apenas a qualidade técnica ideal da fonte audiovisual, mas também as condições reais em que os conteúdos são produzidos, recebidos, transcodificados e distribuídos por uma plataforma digital. No contexto de um marketplace farmacêutico, essa distinção é particularmente relevante, pois os vídeos avaliados não representam apenas material audiovisual genérico, mas peças de comunicação comercial, educativa e demonstrativa, frequentemente contendo embalagens, textos pequenos, rostos, legendas, elementos gráficos, chamadas promocionais e produtos com superfícies reflexivas.

Dessa forma, um dataset composto exclusivamente por vídeos brutos, não comprimidos ou lossless, embora metodologicamente adequado para avaliar a eficiência intrínseca de codecs, pode não representar adequadamente o comportamento de um sistema de distribuição em condições reais. Em ambientes de marketplace, os vídeos normalmente são recebidos em formatos já comprimidos, como H.264/AVC em MP4, HEVC/H.265 em MOV ou MP4, além de arquivos intermediários de produção, como ProRes ou DNxHR, quando fornecidos por agências ou produtoras. Portanto, a metodologia mais apropriada é adotar uma abordagem híbrida, combinando fontes de alta qualidade com arquivos representativos do fluxo real de ingestão e distribuição.

1. Distinção entre benchmark técnico e avaliação realista de distribuição

A avaliação de codecs pode ser conduzida sob duas perspectivas complementares.

A primeira perspectiva é a avaliação técnica controlada, cujo objetivo é medir a eficiência relativa de diferentes codecs a partir de uma fonte de referência de alta qualidade. Nesse caso, utilizam-se arquivos em formatos como ProRes, DNxHR, FFV1, YUV ou Y4M, que preservam o máximo possível da informação visual original. Esse tipo de teste permite responder à seguinte pergunta: dado um conteúdo-fonte de alta qualidade, qual codec oferece a melhor relação entre bitrate e qualidade visual?

A segunda perspectiva é a avaliação realista de distribuição. Nesse cenário, o objetivo não é medir a eficiência absoluta do codec em condições ideais, mas sim avaliar como o pipeline de distribuição degrada vídeos semelhantes aos que seriam efetivamente recebidos pela plataforma. Essa abordagem é mais próxima da realidade operacional de um marketplace, pois considera vídeos oriundos de agências, criadores, bancos de imagens, dispositivos móveis e ferramentas de edição convencionais. Nesse caso, a pergunta central passa a ser: dado o conteúdo que a plataforma realmente recebe, qual configuração de codec, bitrate e resolução entrega a melhor qualidade perceptual ao usuário final?

Para um marketplace farmacêutico, a segunda perspectiva tende a ser mais relevante do ponto de vista de produto, experiência do usuário e operação. No entanto, a primeira continua sendo importante como referência técnica e como base para métricas objetivas de comparação.

2. Estrutura conceitual do dataset

Recomenda-se que o dataset seja organizado em três camadas principais.

A primeira camada corresponde ao source recebido, isto é, o arquivo na forma mais próxima possível daquela em que chegaria à plataforma. Essa camada deve preservar o vídeo original fornecido por uma agência, criador, banco de vídeo ou dispositivo de captura. Exemplos típicos incluem MP4 H.264, MOV HEVC, arquivos 4K de stock footage ou arquivos ProRes fornecidos por produtoras.

A segunda camada corresponde ao master ou mezzanine, quando disponível. Essa camada deve conter a versão de mais alta qualidade possível do conteúdo, preferencialmente em formatos como ProRes 422 HQ, ProRes 4444, DNxHR HQX, DNxHR 444, FFV1, YUV ou Y4M. Essa versão é importante para comparações full-reference e para análises mais controladas de degradação.

A terceira camada corresponde aos encodes de distribuição, gerados a partir do pipeline da plataforma. Nessa camada, os conteúdos são codificados nos formatos de interesse, como H.264/AVC, H.265/HEVC, AV1, VP9 ou outros codecs industriais, em diferentes resoluções, bitrates e configurações de entrega.

Essa organização permite separar a avaliação da qualidade da fonte, a avaliação da degradação causada pela transcodificação e a comparação entre codecs em condições operacionais realistas.

3. Composição proposta do dataset

Considerando a limitação de 16 vídeos, recomenda-se uma composição balanceada em quatro categorias, com quatro vídeos por categoria. Essa distribuição permite representar os principais tipos de conteúdo esperados em um marketplace farmacêutico, ao mesmo tempo em que mantém o dataset suficientemente compacto para execução recorrente de testes.

A primeira categoria deve contemplar vídeos de produto estático, embalagem e lettering. Esses vídeos são essenciais para avaliar a preservação de detalhes finos, legibilidade de rótulos, bordas de letras, logotipos, superfícies brilhantes, embalagens plásticas, frascos de vidro, blisters e fundos claros. Em um contexto de farmácia, essa categoria é crítica, pois a compreensão visual da embalagem pode influenciar diretamente a confiança do usuário e a percepção de qualidade da experiência de compra.

A segunda categoria deve contemplar vídeos educativos com apresentador ou demonstração de uso. Esses conteúdos representam vídeos explicativos, materiais institucionais, orientações de uso, dermoconsultoria, influenciadores ou profissionais de saúde falando diretamente à câmera. Essa classe é importante para avaliar preservação de pele, cabelo, movimento da boca, expressões faciais, sincronia audiovisual, legendas, produto na mão e ruído em ambientes internos.

A terceira categoria deve contemplar vídeos de perfume, beleza e estética cinematográfica, especialmente com câmera lenta, reflexos, vidro, partículas, sprays, líquidos, bokeh, gradientes e baixa luz. Essa classe é relevante porque combina elementos visualmente complexos e sensíveis à compressão, como transparência, highlights especulares, movimento orgânico e áreas de baixo contraste. Além disso, nesses vídeos é fundamental preservar o frame rate original, pois a conversão indevida de 60, 100 ou 120 fps para 30 fps pode alterar substancialmente a natureza do conteúdo e comprometer a validade do teste.

A quarta categoria deve contemplar vídeos comerciais com cortes rápidos, incluindo criativos de curta duração, anúncios verticais, chamadas promocionais, textos sobrepostos, logotipos, CTAs, transições e packshots finais. Essa categoria é importante para avaliar a capacidade do codec de lidar com mudanças bruscas de cena, keyframes, scene cut detection, overlays gráficos e preservação de texto em movimento.

4. Representatividade dos formatos de origem

Para que o dataset represente adequadamente o mundo real, recomenda-se incluir uma mistura controlada de formatos de origem. Em vez de buscar uma homogeneidade artificial, o dataset deve refletir a diversidade de arquivos que uma plataforma digital tende a receber.

Uma composição plausível incluiria arquivos H.264 em MP4, representando entregas comuns de agências, bancos de vídeo e ferramentas de edição; arquivos HEVC em MOV ou MP4, representando capturas provenientes de smartphones modernos; arquivos ProRes ou DNxHR, representando fluxos profissionais de produção; e vídeos de stock em 4K com bitrate elevado, representando conteúdo licenciado de maior qualidade. Para os vídeos comerciais, recomenda-se ainda criar algumas peças sintéticas a partir de assets licenciados, permitindo controle sobre cortes, textos, CTAs e elementos gráficos.

Essa diversidade é metodologicamente importante porque codecs não se comportam de maneira uniforme diante de diferentes tipos de fonte. Um vídeo ProRes 4:2:2 de alta qualidade impõe desafios distintos de um vídeo HEVC de smartphone, que já pode conter compressão temporal, redução de ruído, sharpening computacional e metadados específicos de cor.

5. Critérios de seleção e exclusão

A representatividade do mundo real não deve ser confundida com baixa qualidade ou ausência de controle. O dataset deve conter arquivos realistas, mas tecnicamente aceitáveis. Devem ser priorizados vídeos em 1080p ou 4K, sem marca d’água, sem artefatos severos prévios, com licença adequada, frame rate conhecido e metadados suficientemente rastreáveis.

Devem ser evitados vídeos provenientes de redes sociais ou plataformas que aplicam recompressão agressiva, como arquivos baixados de YouTube, TikTok, Instagram ou WhatsApp. Esses arquivos podem conter degradações severas, perda de detalhe, macroblocking, variação de frame rate, áudio degradado e metadados inconsistentes. Seu uso poderia enviesar a avaliação, deslocando o foco da análise do codec de distribuição para a análise de artefatos já presentes na fonte.

Também devem ser evitados vídeos com marcas reais de medicamentos, alegações terapêuticas explícitas, propagandas reguladas ou claims sensíveis, salvo quando houver autorização e revisão apropriada. Para fins técnicos, é preferível utilizar produtos genéricos, fictícios, cosméticos, dermocosméticos ou elementos gráficos neutros.

6. Estratégia de avaliação

A avaliação deve combinar métricas objetivas e inspeção visual direcionada. Quando houver um master de alta qualidade, é possível realizar testes full-reference, comparando o vídeo codificado e posteriormente decodificado contra a fonte de referência. Nesse caso, métricas como VMAF, PSNR, SSIM, MS-SSIM, bitrate, tempo de codificação, tempo de decodificação e BD-rate podem ser utilizadas para caracterizar o desempenho dos codecs.

Entretanto, quando o arquivo de origem já é comprimido, a interpretação das métricas muda. A comparação contra o arquivo recebido não mede a fidelidade em relação à cena original capturada, mas sim a degradação adicional introduzida pelo pipeline da plataforma. Essa distinção deve ser explicitamente documentada, pois é central para a validade metodológica do estudo.

Além das métricas objetivas, recomenda-se uma avaliação visual manual orientada por critérios específicos do domínio. Para vídeos de produto, devem ser avaliadas legibilidade de rótulos, preservação de logotipos, bordas de texto, cores da embalagem e artefatos em fundos claros. Para vídeos com apresentador, devem ser avaliadas pele, cabelo, movimento labial, sincronia e legibilidade de legendas. Para vídeos de perfume e beleza, devem ser avaliados banding, reflexos, partículas, vidro, fumaça, bokeh e gradientes. Para comerciais, devem ser avaliados artefatos após cortes, estabilidade de textos, preservação de CTAs, logos e packshots finais.

7. Organização dos arquivos e metadados

A organização do dataset deve garantir reprodutibilidade. Cada vídeo deve ser acompanhado de um manifesto com metadados técnicos, origem, licença e características de conteúdo. Esse manifesto deve registrar, no mínimo, identificador do vídeo, categoria, descrição, fonte, licença, resolução, frame rate, codec original, bitrate, container, pixel format, bit depth, color space, presença de áudio, presença de rosto, texto, logo, legenda, produto, slow motion, cortes rápidos e orientação do vídeo.

Uma estrutura de diretórios adequada seria:

dataset_marketplace_16/
  sources_received/
  mezzanine_if_available/
  normalized_for_codec_tests/
  delivery_encodes/
    av1/
    h265/
    h264/
  decoded_for_metrics/
    av1/
    h265/
    h264/
  metadata/
    manifest.csv
    ffprobe_json/
    visual_review.csv

Essa estrutura preserva a separação entre fonte recebida, versões intermediárias, fontes normalizadas, encodes finais, arquivos decodificados para métricas e documentação.

8. Conclusão metodológica

Para um marketplace farmacêutico, um dataset representativo não deve ser construído apenas a partir de fontes brutas ou academicamente ideais. Embora vídeos lossless, YUV, Y4M, FFV1, ProRes e DNxHR sejam fundamentais para testes controlados, eles não refletem integralmente a realidade operacional de uma plataforma de distribuição. O dataset deve, portanto, combinar fontes de alta qualidade com arquivos realistas de ingestão.

A proposta mais robusta é construir um dataset compacto de 16 conteúdos, distribuídos em quatro categorias: produto e embalagem, educativo com apresentador, beleza/perfume cinematográfico e comerciais com cortes rápidos. Para cada conteúdo, deve-se preservar o arquivo recebido, manter o melhor master disponível e gerar encodes de distribuição em diferentes codecs e bitrates. Essa abordagem permite avaliar tanto a eficiência técnica dos codecs quanto sua adequação prática ao cenário real de distribuição de vídeos em marketplace.

Em síntese, o dataset deve representar a realidade do ecossistema de produção e distribuição, mas preservar rigor metodológico suficiente para medir, comparar e interpretar a degradação introduzida por cada etapa do pipeline audiovisual.