=== jiv-about
title: About Jiv, your helper
q: what can jiv do | who are you | what can you do | help | how do i use the assistant | can you deploy for me | does jiv work offline
tags: jiv help assistant bot chat offline can do actions
hi: tum kya kar sakte ho | jiv kya hai | तुम क्या कर सकते हो | मदद
go: Open Jiv -> #/jiv
screen: jiv
---
I'm **Jiv**, Rover's helper. I work **offline** - no internet or AI account needed.

- **Answer questions** about every screen, domains, DNS, HTTPS, servers, GoDaddy, Cloudflare, Netlify and more.
- **Find problems**: ask *"why is my site down?"* or *"why does my domain not work?"* and I check your real setup.
- **Do things for you**: *"deploy bakery"*, *"roll back shop"*, *"create a project called portfolio"*, *"open hosting accounts"*. Anything that changes a server or DNS shows a **confirm card** first.
- **Understand Hindi/Hinglish**: *"meri site band kyun hai?"*

Want smarter free-form answers? Download the **offline brain** in Settings > Jiv (runs on this PC).
@hi
मैं **Jiv** हूँ, Rover का सहायक। मैं **ऑफ़लाइन** काम करता हूँ।

- हर स्क्रीन, डोमेन, DNS, HTTPS, सर्वर के बारे में सवालों के जवाब।
- **समस्या ढूँढना**: पूछें *"meri site down kyun hai?"*
- **काम करना**: *"bakery deploy karo"*, *"shop rollback karo"*। सर्वर या DNS बदलने से पहले मैं पुष्टि माँगता हूँ।

=== offline-brain
title: Jiv's offline brain (local AI on this PC)
q: download offline brain | offline ai model | local ai | make jiv smarter | llama | which model does jiv use | how much ram does the brain need | brain download size
tags: brain offline model llama qwen download ram size local ai gguf smarter
hi: offline brain kaise download kare | local ai | ऑफ़लाइन ब्रेन
go: Settings > Jiv -> #/settings/jiv
do: open_page page=settings/jiv
screen: jiv-settings
---
Jiv always works with built-in answers. The **offline brain** adds free-form understanding and writing, still 100% on this PC:

1. Open **Settings > Jiv** and click **Download offline brain**.
2. Choose **Standard** (Qwen3 4B, 2.5 GB download, needs about 4 GB free RAM - best for 8 GB+ PCs) or **Light** (Qwen3 1.7B, 1.1 GB, about 2 GB RAM - for older laptops). Rover recommends one for your PC.
3. Rover downloads the llama.cpp engine (25 MB) and the model, checks their fingerprints (sha256) and starts it in the background. Nothing leaves your PC when you chat.

Already have the files (e.g. from a USB stick)? Use **Use files I already have**. Both are Apache-2.0 / MIT licensed - free for business use.
@hi
**Settings > Jiv** में **Download offline brain** दबाएँ। **Standard** (2.5 GB, 8 GB+ PC या 4 GB ग्राफ़िक्स कार्ड) या **Light** (1.1 GB, पुराने लैपटॉप) चुनें। सब कुछ इसी PC पर चलता है; बिना इंटरनेट के लिए ऑफ़लाइन पैक या Offline (full) इंस्टॉलर इस्तेमाल करें।

=== ai-providers
title: Connect a cloud AI or another local AI app
q: connect openai | use claude | use gemini | connect ollama | lm studio | gpt4all | jan ai | api key for ai | cloud ai
tags: ai openai claude anthropic gemini grok ollama lmstudio jan gpt4all cloud local provider key
hi: openai kaise jode | ai key | एआई जोड़ें
go: Connect AI -> #/settings/ai
screen: ai
---
Jiv detects local AI apps automatically (Ollama, LM Studio, Jan, GPT4All, llama.cpp) - pick them in the model list on the Jiv page.

Cloud AI (OpenAI, Anthropic, Google Gemini, xAI, OpenRouter...) is **optional**: connect it in **Settings > Connect AI**. When you use it, Rover sends your question plus a short summary of your projects (names, statuses) - **never passwords, tokens or keys**.

=== jiv-actions
title: What Jiv can do for you (actions)
q: what actions can jiv do | can you create a project | can you restart my site | can you add a domain | do it for me | automate
tags: actions automate do create deploy rollback restart domain backup check confirm
hi: kya tum deploy kar sakte ho | मेरे लिए कर दो
go: Open Jiv -> #/jiv
screen: jiv
---
Ask in plain words, for example:

- *"create a project called portfolio"* - *"deploy bakery"* (from its folder) - *"pull latest for shop"*
- *"restart bakery"* - *"roll back bakery"*
- *"add domain mybakery.com to bakery"* - *"check the domain of bakery"*
- *"back up now"* - *"check all my sites"*
- *"open hosting accounts"*, *"open settings"*, *"switch to the server Hetzner 2"*

Read-only checks run straight away. Anything that changes a server, DNS or costs money shows a **confirm card** - nothing happens until you click **Yes, do it**.

=== jiv-privacy
title: Is my chat private?
q: is my chat private | where is chat history stored | clear chat history | does jiv send my data | does jiv send my chat to the cloud | is jiv private
tags: chat history privacy clear local data
hi: chat kahan save hoti hai
go: Settings > Jiv -> #/settings/jiv
screen: jiv-settings
---
Chat history is saved only on this PC (in Rover's settings folder). Clear it any time in **Settings > Jiv > Clear chat history**. With the built-in answers or the offline brain, nothing leaves your PC. With a cloud AI you chose, only your question and a short project summary are sent - never secrets.
