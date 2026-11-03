FROM python:3.12-slim
RUN apt-get update && apt-get install -y --no-install-recommends openssh-client rsync \
    && rm -rf /var/lib/apt/lists/*
RUN pip install --no-cache-dir "ansible-core>=2.17,<2.18" "cryptography>=43" \
    && ansible-galaxy collection install "community.crypto:>=2.22,<3" "community.general:>=9,<10" "ansible.posix:>=1.6,<2"
ENV ANSIBLE_CONFIG=/work/deploy/ansible/ansible.cfg
COPY entrypoint.sh /entrypoint.sh
ENTRYPOINT ["/entrypoint.sh"]
