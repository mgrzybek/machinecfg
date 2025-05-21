DRAWIO_SVG_OPTS=--format svg --embed-svg-images
DRAWIO=draw.io

##############################################################################
# Documentation

all: architecture.svg ## Export the whole pages

# Used to trigger svg targets when this file is changed
architecture.drawio:	# Hidden target

architecture.svg: architecture.drawio ## Export the architecture page to SVG
	$(DRAWIO) $(DRAWIO_SVG_OPTS) \
		--export architecture.drawio \
		--page-index 0 \
		--output architecture.svg

##############################################################################
# Help

.PHONY: help
help: ## This help message
	@awk -F: \
		'$$1 ~/^[a-z\._]/ && $$2~/##/ {gsub(/: .*?\s*##/, "\t");print $0}' \
		Makefile \
	| expand -t22 \
	| sort
